package engine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

func near(t *testing.T, got, want, tol float64, what string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.4f, want %.4f (± %g)", what, got, want, tol)
	}
}

func TestSunPosition(t *testing.T) {
	// Highest sun of the day at Sydney on the December solstice: 90 - (33.87 - 23.44) ≈ 79.6°.
	best := Sun{}
	for h := 11.0; h < 14; h += 1.0 / 60 {
		if s := SunAt(-33.87, 151.21, 11, 2026, 12, 21, h); s.Elevation > best.Elevation {
			best = s
		}
	}
	near(t, best.Elevation, 79.6, 0.3, "Sydney solstice noon elevation")
	near(t, math.Mod(best.Azimuth+180, 360)-180, 0, 3, "the noon sun is due north from Sydney")

	// Equator at the March equinox: nearly overhead at solar noon, rising in the east, setting in the west.
	top := 0.0
	for h := 11.0; h < 13; h += 1.0 / 60 {
		top = math.Max(top, SunAt(0, 0, 0, 2026, 3, 20, h).Elevation)
	}
	if top < 89 {
		t.Errorf("equinox noon at the equator: %.2f°", top)
	}
	near(t, SunAt(0, 0, 0, 2026, 3, 20, 8).Azimuth, 90, 1.5, "morning sun in the east")
	near(t, SunAt(0, 0, 0, 2026, 3, 20, 16).Azimuth, 270, 1.5, "afternoon sun in the west")

	// London midsummer noon: 90 - (51.5 - 23.44) ≈ 61.9°, due south.
	ln := Sun{}
	for h := 12.0; h < 14.5; h += 1.0 / 60 {
		if s := SunAt(51.5, -0.13, 1, 2026, 6, 21, h); s.Elevation > ln.Elevation {
			ln = s
		}
	}
	near(t, ln.Elevation, 61.9, 0.3, "London midsummer noon elevation")
	near(t, ln.Azimuth, 180, 3, "and due south")

	if DirectBeam(0.5) != 0 || DirectBeam(30) >= DirectBeam(60) || DirectBeam(90) > 1100 {
		t.Error("direct beam: zero at the horizon, stronger higher up, below the solar constant")
	}
}

func TestTemperatureCurve(t *testing.T) {
	near(t, temperature(15, 38), 38, 1e-9, "hottest at 3 pm")
	near(t, temperature(5, 38), 26, 1e-9, "coolest at 5 am")
	if temperature(12, 38) <= temperature(9, 38) || temperature(18, 38) >= temperature(15, 38) {
		t.Error("warms through the morning, cools through the evening")
	}
}

func TestRaysAndCrowns(t *testing.T) {
	box := Box{X: 0, Y: 0, W: 10, D: 10, H: 10}
	up := [3]float64{0, 0, 1}
	if !rayHitsBox(5, 5, 1.5, up, box) {
		t.Error("straight up from inside a building's footprint hits it")
	}
	if rayHitsBox(-1, 5, 1.5, up, box) {
		t.Error("straight up beside it doesn't")
	}
	s := math.Sqrt(0.5)
	if !rayHitsBox(-3, 5, 1.5, [3]float64{s, 0, s}, box) || rayHitsBox(-3, 5, 1.5, [3]float64{-s, 0, s}, box) {
		t.Error("a 45° ray hits a building towards the sun, not one behind")
	}

	c := Canopy{X: 0, Y: 0, Z: 10, R: 3, V: 2, Through: 0.2}
	near(t, c.Transmit(0, 0, 1.5, 0, 0, 1), 0.2, 1e-9, "straight through the middle")
	near(t, c.Transmit(10, 0, 1.5, 0, 0, 1), 1, 0, "missing the crown")
	near(t, c.Transmit(0, 0, 20, 0, 0, 1), 1, 0, "a crown below the walker doesn't shade")
	off := c.Transmit(2.5, 0, 1.5, 0, 0, 1)
	if off <= 0.2 || off >= 1 {
		t.Errorf("an off-centre ray goes through less leaf: %.3f", off)
	}
	brush, _ := SpeciesByID("brush-box")
	if brush.HeightAt(0) != plantedHeight || brush.HeightAt(brush.HalfYears) <= brush.HeightAt(1) || brush.HeightAt(200) > brush.Height {
		t.Error("trees start at planting size, grow, and never pass their mature height")
	}
	near(t, brush.HeightAt(brush.HalfYears), (plantedHeight+brush.Height)/2, 1e-9, "half-way after HalfYears")
	plane, _ := SpeciesByID("plane")
	if plane.CanopyAt(0, 0, 10, true).Through <= plane.CanopyAt(0, 0, 10, false).Through {
		t.Error("a deciduous tree lets more sun through in winter")
	}
}

func scene(t *testing.T, text string) *Scene {
	t.Helper()
	s, err := ParseScene(strings.NewReader(text))
	if err != nil {
		t.Fatalf("scene: %v", err)
	}
	return s
}

// One footpath running east-west, a building on its north side.
const simple = `{"latitude": -33.9, "longitude": 151.2, "utcOffset": 11, "date": "2026-01-15",
 "buildings": [{"x": -20, "y": 3, "w": 40, "d": 10, "h": 12}],
 "walks": [{"x0": -10, "y0": 0, "x1": 10, "y1": 3}],
 "sites": [{"id": "A", "x": -5, "y": -1}, {"id": "B", "x": 5, "y": -1}]}`

func TestBuildingShade(t *testing.T) {
	s := scene(t, simple)
	g := MakeGrid(s)
	e := Measure(s, g, false)
	if len(g.Cells) != 60 || g.CellSize != 1 {
		t.Fatalf("grid: %d cells of %g m", len(g.Cells), g.CellSize)
	}
	// In Sydney the summer noon sun is north and high: the building to the north shades the
	// footpath beside it in the early afternoon.
	k := -1
	for i, st := range g.Steps {
		if math.Abs(st.Hour-13.125) < 0.01 {
			k = i
		}
	}
	shaded := 0
	for c := range g.Cells {
		if e.Sunlit[e.Index(c, k)] == 0 {
			shaded++
		}
	}
	if shaded == 0 {
		t.Error("a 12 m building just north of the path shades some of it at 1 pm in Sydney")
	}
	// The same street turned so the building is south of the path: no shade at noon.
	turned := scene(t, strings.Replace(simple, `"date"`, `"bearing": 180, "date"`, 1))
	e2 := Measure(turned, MakeGrid(turned), false)
	for c := range g.Cells {
		if e2.Sunlit[e2.Index(c, k)] == 0 {
			t.Fatal("a building south of the path can't shade it at noon in a southern summer")
		}
	}
}

// removed returns how much sun a set of candidates removes, by applying them in turn.
func removed(e *Exposure, cands []candidate, set []int) float64 {
	now := append([]float64(nil), e.Sunlit...)
	for _, c := range set {
		cands[c].apply(now)
	}
	return e.Total(e.Sunlit) - e.Total(now)
}

func randomScene(rng *rand.Rand) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"latitude": %.2f, "longitude": 150, "utcOffset": 10, "date": "2026-02-%02d", "stepMinutes": 30,
 "budget": %d, "minSpacing": 3, "maxShare": 1, "species": ["water-gum", "chinese-elm"],
 "walks": [{"x0": -12, "y0": -2, "x1": 12, "y1": 2}],`, -40+rng.Float64()*30, 1+rng.IntN(27), 2+rng.IntN(2))
	b.WriteString(`"buildings": [`)
	for i := 0; i < 2; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"x": %.1f, "y": %.1f, "w": 6, "d": 6, "h": %.1f}`, -15+rng.Float64()*30, []float64{-12, 6}[i], 4+rng.Float64()*10)
	}
	b.WriteString(`], "sites": [`)
	for i := 0; i < 6; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id": "S%d", "x": %.1f, "y": %.1f}`, i, -10+rng.Float64()*20, []float64{-3, 3}[i%2])
	}
	b.WriteString("]}")
	return b.String()
}

// Exhaustive search over every feasible plan on small random streets: the greedy plan must be
// within (1 - 1/e) of the best, the upper bound must never be below the best, and the lazy
// greedy must pick exactly what a plain greedy picks.
func TestGreedyAgainstExhaustiveSearch(t *testing.T) {
	rng := rand.New(rand.NewPCG(26, 10))
	worst, matched := 1.0, 0
	const trials = 40
	for trial := 0; trial < trials; trial++ {
		s := scene(t, randomScene(rng))
		e := Measure(s, MakeGrid(s), false)
		p := must(Optimise(s, e))
		cands := must(candidates(s, e, float64(s.Year), false))
		best := 0.0
		var walk func(start int, set []int)
		walk = func(start int, set []int) {
			if v := removed(e, cands, set); v > best {
				best = v
			}
			if len(set) == s.Budget {
				return
			}
			for c := start; c < len(cands); c++ {
				if feasible(s, set, cands, c) {
					walk(c+1, append(set, c))
				}
			}
		}
		walk(0, nil)
		got := p.Before - p.After
		if got < (1-1/math.E)*best-1e-9 {
			t.Fatalf("trial %d: greedy %.4f below (1-1/e) x best %.4f", trial, got, best)
		}
		if p.UpperBound < best-1e-9 {
			t.Fatalf("trial %d: bound %.4f below the true best %.4f", trial, p.UpperBound, best)
		}
		if best > 0 {
			worst = math.Min(worst, got/best)
			if got >= best-1e-9 {
				matched++
			}
		}

		// Plain greedy: recompute every gain every round.
		now := append([]float64(nil), e.Sunlit...)
		var chosen []int
		for len(chosen) < s.Budget {
			pick, pickGain := -1, 1e-12
			for c := range cands {
				if !feasible(s, chosen, cands, c) {
					continue
				}
				if g := cands[c].gain(e, now); g > pickGain+1e-15 {
					pick, pickGain = c, g
				}
			}
			if pick < 0 {
				break
			}
			cands[pick].apply(now)
			chosen = append(chosen, pick)
		}
		if len(chosen) != len(p.Picks) {
			t.Fatalf("trial %d: plain greedy picked %d trees, lazy %d", trial, len(chosen), len(p.Picks))
		}
		for i, c := range chosen {
			if s.Sites[cands[c].site].ID != p.Picks[i].Site || cands[c].species.ID != p.Picks[i].Species {
				t.Fatalf("trial %d: pick %d differs between plain and lazy greedy", trial, i)
			}
		}
	}
	t.Logf("greedy matched the exhaustive best in %d of %d streets; worst %.1f%% of the best", matched, trials, worst*100)
	if matched < trials/2 {
		t.Errorf("greedy should usually be optimal on small streets (matched %d of %d)", matched, trials)
	}
}

// Removed sun is monotone and submodular: adding a tree never hurts, and a tree helps less once
// others are planted.
func TestSubmodular(t *testing.T) {
	s := scene(t, string(must(os.ReadFile("../scenes/terrace-street.json"))))
	e := Measure(s, MakeGrid(s), false)
	cands := must(candidates(s, e, 10, false))
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := 0; trial < 200; trial++ {
		a := []int{rng.IntN(len(cands))}
		b := append([]int{rng.IntN(len(cands)), rng.IntN(len(cands))}, a...) // a is a subset of b
		x := rng.IntN(len(cands))
		gainA := removed(e, cands, append(append([]int{}, a...), x)) - removed(e, cands, a)
		gainB := removed(e, cands, append(append([]int{}, b...), x)) - removed(e, cands, b)
		if gainA < -1e-9 || gainB < -1e-9 {
			t.Fatal("adding a tree never makes the street sunnier")
		}
		if gainB > gainA+1e-9 {
			t.Fatalf("a tree helped more in the larger plan: %.6f > %.6f", gainB, gainA)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestBundledScenes(t *testing.T) {
	for _, name := range []string{"terrace-street", "city-avenue", "town-square"} {
		s := scene(t, string(must(os.ReadFile("../scenes/"+name+".json"))))
		r := must(Run(s))
		p := r.Plan
		if len(p.Picks) == 0 || p.Reduction <= 0.05 || p.Guarantee <= 0.4 || p.Guarantee > 1 {
			t.Errorf("%s: %d picks, %.3f removed, guarantee %.3f", name, len(p.Picks), p.Reduction, p.Guarantee)
		}
		per := map[string]int{}
		for i, a := range p.Picks {
			per[a.Species]++
			site := s.Sites[siteIndex(s, a.Site)]
			sp, _ := SpeciesByID(a.Species)
			if site.MaxHeight > 0 && sp.Height > site.MaxHeight {
				t.Errorf("%s: a %s under a %g m height limit", name, sp.Name, site.MaxHeight)
			}
			for _, b := range p.Picks[i+1:] {
				if math.Hypot(a.X-b.X, a.Y-b.Y) < s.MinSpacing {
					t.Errorf("%s: %s and %s are closer than %g m", name, a.Site, b.Site, s.MinSpacing)
				}
			}
			if i > 0 && a.Gain > p.Picks[i-1].Gain+1e-9 {
				t.Errorf("%s: gains shrink as the plan grows (submodularity)", name)
			}
		}
		for sp, n := range per {
			if n > MaxPerSpecies(s) {
				t.Errorf("%s: %d %s trees, over the species limit", name, n, sp)
			}
		}
		for i := 1; i < len(r.Growth); i++ {
			if r.Growth[i].Reduction < r.Growth[i-1].Reduction {
				t.Errorf("%s: shade shrinks as trees grow", name)
			}
		}
		if r.Seasons.WinterLost < 0 || r.Seasons.WinterLost > 1 || len(r.CellX) != len(r.After) {
			t.Errorf("%s: odd seasons or cell arrays", name)
		}
		for c := range r.After {
			if r.After[c] > r.Before[c]+1e-9 {
				t.Fatalf("%s: a cell got sunnier after planting", name)
			}
		}
		if !strings.Contains(r.Text(), "at least") {
			t.Errorf("%s: the report states the guarantee", name)
		}
	}
}

func siteIndex(s *Scene, id string) int {
	for i, x := range s.Sites {
		if x.ID == id {
			return i
		}
	}
	return -1
}

func TestDeciduousTreesKeepWinterSun(t *testing.T) {
	base := `{"latitude": -33.9, "longitude": 151.2, "utcOffset": 11, "date": "2026-01-15", "budget": 1, "targetYear": 20,
 "walks": [{"x0": -10, "y0": -10, "x1": 10, "y1": 10}], "sites": [{"id": "A", "x": 0, "y": 0}], "species": ["SPECIES"]}`
	run := func(sp string) Seasons {
		return must(Run(scene(t, strings.Replace(base, "SPECIES", sp, 1)))).Seasons
	}
	elm, box := run("chinese-elm"), run("brush-box")
	if elm.WinterLost >= box.WinterLost {
		t.Errorf("a deciduous elm should take less winter sun (%.3f) than an evergreen brush box (%.3f)", elm.WinterLost, box.WinterLost)
	}
}

func TestSceneValidation(t *testing.T) {
	cases := map[string]string{
		`{"walks": [], "sites": []}`: "date",
		`[]`:                         "wrong type",
		simple + ` {}`:               "extra data",
		strings.Replace(simple, `"latitude"`, `"lattitude"`, 1):            "doesn't know",
		strings.Replace(simple, `2026-01-15`, `2026-02-30`, 1):             "real date",
		strings.Replace(simple, `"h": 12`, `"h": 9000`, 1):                 "building 1",
		strings.Replace(simple, `"x1": 10`, `"x1": -20`, 1):                "x0 must be less",
		strings.Replace(simple, `"id": "B"`, `"id": "A"`, 1):               "used twice",
		strings.Replace(simple, `"id": "B"`, `"id": "<b>"`, 1):             "ids are",
		strings.Replace(simple, `"date"`, `"name": "<script>", "date"`, 1): "the name uses",
		strings.Replace(simple, `"date"`, `"budget": 500, "date"`, 1):      "budget",
		strings.Replace(simple, `"date"`, `"species": ["oak"], "date"`, 1): "unknown species",
		strings.Replace(simple, `"date"`, `"maxTemp": 99, "date"`, 1):      "maxTemp",
		strings.Replace(simple, `"x0": -10`, `"x0": -1e9`, 1):              "walk 1",
		`{"latitude": 1e999}`:                "wrong type",
		`{"latitude": 1,`:                    "not valid JSON",
		strings.Repeat(" ", MaxSceneBytes+1): "larger than 256 KB",
	}
	for text, want := range cases {
		_, err := ParseScene(strings.NewReader(text))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
		if err != nil && strings.Contains(err.Error(), "<") {
			t.Errorf("an error echoed the input: %v", err)
		}
	}
	// A big plaza gets coarser cells so the work stays bounded.
	big := strings.Replace(simple, `"walks": [{"x0": -10, "y0": 0, "x1": 10, "y1": 3}]`,
		`"walks": [{"x0": -100, "y0": -100, "x1": 100, "y1": 99}]`, 1)
	g := MakeGrid(scene(t, big))
	if len(g.Cells) > MaxCells || g.CellSize < 2 {
		t.Errorf("big walks: %d cells of %g m", len(g.Cells), g.CellSize)
	}
}

func FuzzParseScene(f *testing.F) {
	f.Add(simple)
	f.Add(`{"walks":[{"x0":0,"y0":0,"x1":1,"y1":1}],"sites":[{"id":"a","x":0,"y":0}],"date":"2026-01-01"}`)
	f.Fuzz(func(t *testing.T, text string) {
		s, err := ParseScene(strings.NewReader(text))
		if err != nil {
			return
		}
		if len(s.Walks) == 0 || len(s.Sites) == 0 || s.Budget < 1 {
			t.Fatal("a scene that passed validation is incomplete")
		}
	})
}

func TestHugeScenesAreRefusedNotRun(t *testing.T) {
	var sites []string
	for i := 0; i < MaxSites; i++ {
		sites = append(sites, fmt.Sprintf(`{"id": "S%d", "x": %d, "y": %d}`, i, -40+(i%9)*10, -40+(i/9)*10))
	}
	text := `{"latitude": -33.9, "longitude": 151.2, "utcOffset": 11, "date": "2026-01-15", "fromHour": 5, "toHour": 21,
 "stepMinutes": 10, "targetYear": 40, "walks": [{"x0": -45, "y0": -45, "x1": 45, "y1": 45}], "sites": [` + strings.Join(sites, ",") + `]}`
	s := scene(t, text)
	g := MakeGrid(s)
	if len(g.Cells)*len(g.Steps) > MaxCellSteps {
		t.Fatalf("the grid wasn't coarsened: %d cells x %d steps", len(g.Cells), len(g.Steps))
	}
	r, err := Run(s)
	if err != nil && err != ErrTooBig {
		t.Fatalf("unexpected error %v", err)
	}
	if err == nil && len(r.Plan.Picks) == 0 {
		t.Fatal("a big scene that runs still gives a plan")
	}
}
