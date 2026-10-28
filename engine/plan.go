package engine

import (
	"container/heap"
	"math"
	"sort"
)

// candidate is one species at one site: which cell-steps its crown would shade, and how much
// sun it would let through at each.
type candidate struct {
	site    int
	species Species
	index   []int32   // flat cell x step indexes it shades
	through []float32 // fraction of sun let through at each
}

// Pick is a tree in the plan.
type Pick struct {
	Site      string  `json:"site"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Species   string  `json:"species"`
	Gain      float64 `json:"gain"`      // heat-weighted sun it removes, given the trees picked before it
	GainShare float64 `json:"gainShare"` // that as a share of the street's total exposure
	Order     int     `json:"order"`
}

// Plan is the planner's answer.
type Plan struct {
	Picks       []Pick  `json:"picks"`
	Before      float64 `json:"before"`      // heat-weighted sun on the walks with today's shade
	After       float64 `json:"after"`       // ... with the new trees at the target year
	Reduction   float64 `json:"reduction"`   // share of Before removed
	UpperBound  float64 `json:"upperBound"`  // no plan of this size can remove more than this (same units)
	Guarantee   float64 `json:"guarantee"`   // the plan's removal as a share of that bound (a floor on how close to the best it is)
	Evaluations int     `json:"evaluations"` // marginal gains computed (lazy greedy saves most of them)
	Candidates  int     `json:"candidates"`

	through []float64 // per cell x step, after planting
}

// crowns builds every allowed (site, species) candidate at the target year.
// MaxEntries caps the shade records kept for all candidates (about 8 bytes each), so a huge scene
// is refused with an explanation instead of exhausting memory.
const MaxEntries = 12_000_000

// ErrTooBig is returned when a scene would need more work than one run allows.
var ErrTooBig = &InputError{Msg: "this scene is too big for one run: use fewer sites, fewer species, shorter hours or a longer step"}

func candidates(s *Scene, e *Exposure, years float64, winter bool) ([]candidate, error) {
	var out []candidate
	total := 0
	steps := e.Grid.Steps
	for si, site := range s.Sites {
		for _, sp := range s.AllowedSpecies() {
			if site.MaxHeight > 0 && sp.Height > site.MaxHeight {
				continue
			}
			cr := sp.CanopyAt(site.X, site.Y, years, winter)
			cand := candidate{site: si, species: sp}
			// Only cells within reach of the crown's shadow can be affected: the shadow of a crown
			// with top at height Z+V falls at most (Z+V-eye)/tan(elev) + R away.
			minElev := 90.0
			for _, st := range steps {
				minElev = math.Min(minElev, st.Sun.Elevation)
			}
			reach := cr.R + (cr.Z+cr.V-EyeHeight)/math.Tan(math.Max(minElev, 3)*deg) + e.Grid.CellSize
			for c, cell := range e.Grid.Cells {
				if math.Hypot(cell.X-site.X, cell.Y-site.Y) > reach {
					continue
				}
				for k, st := range steps {
					i := e.Index(c, k)
					if e.Value[i] == 0 || e.Sunlit[i] == 0 {
						continue
					}
					tr := cr.Transmit(cell.X, cell.Y, EyeHeight, st.Dir[0], st.Dir[1], st.Dir[2])
					if tr < 0.999 {
						cand.index = append(cand.index, int32(i))
						cand.through = append(cand.through, float32(tr))
						if total++; total > MaxEntries {
							return nil, ErrTooBig
						}
					}
				}
			}
			if len(cand.index) > 0 {
				out = append(out, cand)
			}
		}
	}
	return out, nil
}

// gain is how much heat-weighted sun a candidate would remove, given what gets through now.
func (c *candidate) gain(e *Exposure, now []float64) float64 {
	g := 0.0
	for k, i := range c.index {
		g += e.Value[i] * now[i] * (1 - float64(c.through[k]))
	}
	return g
}

func (c *candidate) apply(now []float64) {
	for k, i := range c.index {
		now[i] *= float64(c.through[k])
	}
}

// A max-heap of candidates by their last known (possibly stale) gain.
type entry struct {
	cand  int
	gain  float64
	round int // the plan size when this gain was computed
}
type queue []entry

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	return q[i].gain > q[j].gain || (q[i].gain == q[j].gain && q[i].cand < q[j].cand)
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(entry)) }
func (q *queue) Pop() any     { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

// feasible reports whether a site can take a new tree given the trees chosen so far.
func feasible(s *Scene, chosen []int, cands []candidate, c int) bool {
	site := s.Sites[cands[c].site]
	for _, o := range chosen {
		other := s.Sites[cands[o].site]
		if cands[o].site == cands[c].site || math.Hypot(site.X-other.X, site.Y-other.Y) < s.MinSpacing {
			return false
		}
	}
	for _, t := range s.Existing {
		if math.Hypot(site.X-t.X, site.Y-t.Y) < s.MinSpacing/2 {
			return false
		}
	}
	// Species mix: no more than MaxShare of the budget from one species (at least one allowed).
	same := 0
	for _, o := range chosen {
		if cands[o].species.ID == cands[c].species.ID {
			same++
		}
	}
	return same < MaxPerSpecies(s)
}

// MaxPerSpecies is how many new trees one species may supply.
func MaxPerSpecies(s *Scene) int {
	return max(1, int(math.Floor(s.MaxShare*float64(s.Budget)+1e-9)))
}

// Optimise picks up to Budget trees by lazy greedy selection.
//
// Removed sun is a monotone submodular function of the set of trees: adding a tree never hurts,
// and a tree helps less when others already shade the same cells (sun is multiplied by each
// crown's transmission, so overlapping crowns have diminishing returns). For such functions the
// greedy choice is provably within (1 - 1/e) ≈ 63% of the best plan of the same size, and lazy
// evaluation is exact: a gain can only shrink as the plan grows, so a stale gain at the top of
// the queue that is still the best after recomputing is truly the best.
func Optimise(s *Scene, e *Exposure) (*Plan, error) {
	cands, err := candidates(s, e, float64(s.Year), false)
	if err != nil {
		return nil, err
	}
	now := append([]float64(nil), e.Sunlit...)
	before := e.Total(now)
	p := &Plan{Before: before, Candidates: len(cands), Picks: []Pick{}}

	q := make(queue, 0, len(cands))
	for i := range cands {
		q = append(q, entry{cand: i, gain: cands[i].gain(e, now), round: 0})
		p.Evaluations++
	}
	heap.Init(&q)
	var chosen []int
	for len(chosen) < s.Budget && q.Len() > 0 {
		top := heap.Pop(&q).(entry)
		if !feasible(s, chosen, cands, top.cand) {
			continue // its site, a neighbour or its species quota was taken: it can never become feasible again
		}
		if top.round != len(chosen) {
			top.gain = cands[top.cand].gain(e, now)
			top.round = len(chosen)
			p.Evaluations++
			heap.Push(&q, top)
			continue
		}
		if top.gain <= 1e-12 {
			break
		}
		c := cands[top.cand]
		c.apply(now)
		chosen = append(chosen, top.cand)
		site := s.Sites[c.site]
		p.Picks = append(p.Picks, Pick{Site: site.ID, X: site.X, Y: site.Y, Species: c.species.ID, Gain: top.gain,
			GainShare: share(top.gain, before), Order: len(chosen)})
	}
	p.After = e.Total(now)
	p.Reduction = share(before-p.After, before)
	p.through = now

	// A data-dependent bound on the best possible plan. For any plan OPT of up to Budget trees,
	// removed(OPT) <= removed(OPT + ours) <= removed(ours) + the sum over OPT's trees of the gain
	// each would still add to ours (submodularity). OPT has at most one tree per site, so that sum
	// is at most the Budget largest per-site best gains.
	bestAt := map[int]float64{}
	for i := range cands {
		g := cands[i].gain(e, now)
		if g > bestAt[cands[i].site] {
			bestAt[cands[i].site] = g
		}
	}
	var gains []float64
	for _, g := range bestAt {
		gains = append(gains, g)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(gains)))
	ub := before - p.After
	for i := 0; i < s.Budget && i < len(gains); i++ {
		ub += gains[i]
	}
	p.UpperBound = ub
	if ub > 0 {
		p.Guarantee = (before - p.After) / ub
	} else {
		p.Guarantee = 1
	}
	return p, nil
}

func share(a, b float64) float64 {
	if b <= 0 {
		return 0
	}
	return a / b
}

// Evaluate measures a fixed set of trees at a given age and season: how much of the sun on the
// walks they remove compared with the street as it is.
func Evaluate(s *Scene, e *Exposure, picks []Pick, years float64, winter bool) (removed, before float64) {
	now := append([]float64(nil), e.Sunlit...)
	before = e.Total(now)
	steps := e.Grid.Steps
	for _, pk := range picks {
		sp, _ := SpeciesByID(pk.Species)
		cr := sp.CanopyAt(pk.X, pk.Y, years, winter)
		for c, cell := range e.Grid.Cells {
			for k, st := range steps {
				i := e.Index(c, k)
				if now[i] == 0 {
					continue
				}
				now[i] *= cr.Transmit(cell.X, cell.Y, EyeHeight, st.Dir[0], st.Dir[1], st.Dir[2])
			}
		}
	}
	return before - e.Total(now), before
}

// CellTotals sums each cell's heat-weighted sun over the day, as it is and with the plan.
func CellTotals(e *Exposure, through []float64) []float64 {
	n := len(e.Grid.Steps)
	out := make([]float64, len(e.Grid.Cells))
	for c := range out {
		w := e.Grid.Cells[c].Weight
		if w == 0 {
			continue
		}
		t := 0.0
		for k := 0; k < n; k++ {
			i := c*n + k
			t += e.Value[i] * through[i]
		}
		out[c] = t / w // per square metre of footpath, per unit footfall
	}
	return out
}

// Through returns the plan's per cell x step sunlit fraction.
func (p *Plan) Through() []float64 { return p.through }
