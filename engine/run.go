package engine

import (
	"math"
	"sort"
)

// Growth is how much sun the planned trees remove at a given age.
type Growth struct {
	Year      int     `json:"year"`
	Reduction float64 `json:"reduction"`
}

// Seasons compares summer shade won with winter sun lost.
type Seasons struct {
	SummerRemoved float64 `json:"summerRemoved"` // share of summer sun removed at the target year
	WinterLost    float64 `json:"winterLost"`    // share of winter sun the same trees take away
	WinterDate    string  `json:"winterDate"`
}

// Moment is one step of the day, for drawing.
type Moment struct {
	Hour      float64    `json:"hour"`
	Elevation float64    `json:"elevation"`
	Azimuth   float64    `json:"azimuth"`
	Dir       [3]float64 `json:"dir"`
	Weight    float64    `json:"weight"`
	Temp      float64    `json:"temp"`
}

// Crown is a tree's crown at an age, for drawing: centre height, horizontal radius, half-height.
type Crown struct {
	Year int     `json:"year"`
	Z    float64 `json:"z"`
	R    float64 `json:"r"`
	V    float64 `json:"v"`
}

// TreeShape is a planned or existing tree with its crown at each age shown.
type TreeShape struct {
	Site     string  `json:"site,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Species  string  `json:"species"`
	Existing bool    `json:"existing"`
	Crowns   []Crown `json:"crowns"`
}

// Result is everything the web page and the command line show.
type Result struct {
	Scene    *Scene      `json:"scene"`
	CellSize float64     `json:"cellSize"`
	CellX    []float64   `json:"cellX"`
	CellY    []float64   `json:"cellY"`
	Before   []float64   `json:"cellBefore"` // heat-weighted sun per m² over the day, today
	After    []float64   `json:"cellAfter"`  // ... with the plan at the target year
	Moments  []Moment    `json:"moments"`
	Plan     *Plan       `json:"plan"`
	Growth   []Growth    `json:"growth"`
	Seasons  Seasons     `json:"seasons"`
	Species  []Species   `json:"species"`
	Trees    []TreeShape `json:"trees"`
	Years    []int       `json:"years"` // the ages crowns are given for
	Notes    []string    `json:"notes"`
}

// MaxCellSteps caps footpath cells x moments of the day for one run.
const MaxCellSteps = 300_000

// Run does the whole analysis for a checked scene.
func Run(s *Scene) (*Result, error) {
	g := MakeGrid(s)
	if len(g.Cells)*len(g.Steps) > MaxCellSteps {
		return nil, ErrTooBig
	}
	e := Measure(s, g, false)
	plan, err := Optimise(s, e)
	if err != nil {
		return nil, err
	}
	r := &Result{Scene: s, CellSize: g.CellSize, Plan: plan, Species: Catalogue, Notes: []string{}, Trees: []TreeShape{}, Moments: []Moment{}}
	for _, c := range g.Cells {
		r.CellX = append(r.CellX, round(c.X, 2))
		r.CellY = append(r.CellY, round(c.Y, 2))
	}
	r.Before = roundAll(CellTotals(e, e.Sunlit), 4)
	r.After = roundAll(CellTotals(e, plan.Through()), 4)
	for _, st := range g.Steps {
		r.Moments = append(r.Moments, Moment{Hour: st.Hour, Elevation: round(st.Sun.Elevation, 2), Azimuth: round(st.Sun.Azimuth, 2),
			Dir: [3]float64{round(st.Dir[0], 5), round(st.Dir[1], 5), round(st.Dir[2], 5)}, Weight: round(st.Weight, 5), Temp: round(st.Heat, 1)})
	}
	for _, y := range []int{1, 3, 5, 10, 15, 20, 30} {
		removed, before := Evaluate(s, e, plan.Picks, float64(y), false)
		r.Growth = append(r.Growth, Growth{Year: y, Reduction: round(share(removed, before), 4)})
	}
	r.Years = []int{0, 1, 3, 5, 10, 15, 20, 30}
	if s.Year != 1 && s.Year != 3 && s.Year != 5 && s.Year != 10 && s.Year != 15 && s.Year != 20 && s.Year != 30 {
		r.Years = append(r.Years, s.Year)
		sort.Ints(r.Years)
	}
	shape := func(sp Species, x, y, age float64) []Crown {
		var out []Crown
		for _, yr := range r.Years {
			c := sp.CanopyAt(x, y, age+float64(yr), false)
			out = append(out, Crown{Year: yr, Z: round(c.Z, 3), R: round(c.R, 3), V: round(c.V, 3)})
		}
		return out
	}
	for _, pk := range plan.Picks {
		sp, _ := SpeciesByID(pk.Species)
		r.Trees = append(r.Trees, TreeShape{Site: pk.Site, X: pk.X, Y: pk.Y, Species: pk.Species, Crowns: shape(sp, pk.X, pk.Y, 0)})
	}
	for _, t := range s.Existing {
		sp, _ := SpeciesByID(t.Species)
		r.Trees = append(r.Trees, TreeShape{X: t.X, Y: t.Y, Species: t.Species, Existing: true, Crowns: shape(sp, t.X, t.Y, t.Age)})
	}
	w := s.Winter()
	we := Measure(w, MakeGrid(w), true)
	lost, wBefore := Evaluate(w, we, plan.Picks, float64(s.Year), true)
	r.Seasons = Seasons{SummerRemoved: round(plan.Reduction, 4), WinterLost: round(share(lost, wBefore), 4), WinterDate: w.Date}

	if len(g.Steps) == 0 {
		r.Notes = append(r.Notes, "The sun is never more than 2° up in the chosen hours, so there is nothing to shade.")
	}
	if len(plan.Picks) < s.Budget {
		r.Notes = append(r.Notes, "Fewer trees than the budget were placed: the other sites are too close together, or would shade nothing that matters.")
	}
	if plan.Before > 0 && plan.Reduction < 0.02 {
		r.Notes = append(r.Notes, "The street is already well shaded at these hours; new trees change little.")
	}
	return r, nil
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

func roundAll(vs []float64, places int) []float64 {
	for i := range vs {
		vs[i] = round(vs[i], places)
	}
	return vs
}
