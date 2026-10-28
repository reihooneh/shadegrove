package engine

import "math"

// Species describes how a street tree grows and how much sun its canopy lets through.
// The figures are indicative, drawn from typical street-tree guides; a council's own
// species list should take precedence.
type Species struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Botanical   string  `json:"botanical"`
	Height      float64 `json:"height"`    // mature height, m
	Spread      float64 `json:"spread"`    // mature canopy diameter, m
	HalfYears   float64 `json:"halfYears"` // years to reach half its mature size
	Summer      float64 `json:"summer"`    // fraction of direct sun through the middle of the canopy in leaf
	Winter      float64 `json:"winter"`    // ... and in winter (bare branches for deciduous trees)
	Deciduous   bool    `json:"deciduous"`
	CanopyShape float64 `json:"shape"` // canopy height as a fraction of its width (1 = round)
}

// Catalogue is the built-in species list.
var Catalogue = []Species{
	{ID: "brush-box", Name: "Brush Box", Botanical: "Lophostemon confertus", Height: 15, Spread: 10, HalfYears: 12, Summer: 0.15, Winter: 0.15, CanopyShape: 1.1},
	{ID: "plane", Name: "London Plane", Botanical: "Platanus × acerifolia", Height: 22, Spread: 18, HalfYears: 14, Summer: 0.12, Winter: 0.7, Deciduous: true, CanopyShape: 0.9},
	{ID: "chinese-elm", Name: "Chinese Elm", Botanical: "Ulmus parvifolia", Height: 12, Spread: 12, HalfYears: 9, Summer: 0.2, Winter: 0.55, Deciduous: true, CanopyShape: 0.75},
	{ID: "jacaranda", Name: "Jacaranda", Botanical: "Jacaranda mimosifolia", Height: 12, Spread: 12, HalfYears: 8, Summer: 0.25, Winter: 0.6, Deciduous: true, CanopyShape: 0.7},
	{ID: "water-gum", Name: "Water Gum", Botanical: "Tristaniopsis laurina", Height: 8, Spread: 6, HalfYears: 10, Summer: 0.2, Winter: 0.2, CanopyShape: 1.2},
	{ID: "crepe-myrtle", Name: "Crepe Myrtle", Botanical: "Lagerstroemia indica", Height: 6, Spread: 5, HalfYears: 6, Summer: 0.35, Winter: 0.75, Deciduous: true, CanopyShape: 1},
}

// SpeciesByID finds a species in the catalogue.
func SpeciesByID(id string) (Species, bool) {
	for _, s := range Catalogue {
		if s.ID == id {
			return s, true
		}
	}
	return Species{}, false
}

// Canopy is a tree's crown at a given age: an ellipsoid centred at (X, Y, Z) with horizontal
// radius R and vertical semi-axis V.
type Canopy struct {
	X, Y, Z, R, V float64
	Through       float64 // fraction of sun through the middle of the crown
}

const plantedHeight = 2.5 // a typical advanced street-tree stock size, m

// HeightAt is the tree's height a given number of years after planting: it approaches its
// mature height exponentially, reaching half the remaining growth after HalfYears.
func (s Species) HeightAt(years float64) float64 {
	k := math.Ln2 / s.HalfYears
	return plantedHeight + (s.Height-plantedHeight)*(1-math.Exp(-k*years))
}

// CanopyAt builds the crown of a tree at (x, y), aged `years`, in summer or winter.
func (s Species) CanopyAt(x, y, years float64, winter bool) Canopy {
	h := s.HeightAt(years)
	r := s.Spread / 2 * h / s.Height
	// Street trees are pruned to keep a clear trunk over the footpath.
	clear := math.Max(2.4, 0.3*h)
	v := math.Min(r*s.CanopyShape, math.Max(0.5, (h-clear)/2))
	through := s.Summer
	if winter {
		through = s.Winter
	}
	return Canopy{X: x, Y: y, Z: h - v, R: r, V: v, Through: through}
}

// Transmit is the fraction of a sun ray from (px, py, pz) in direction (dx, dy, dz) that gets
// through this crown. Light fades exponentially with the distance travelled through leaves
// (the Beer-Lambert law), scaled so a ray through the very middle lets through Through.
func (c Canopy) Transmit(px, py, pz, dx, dy, dz float64) float64 {
	// Squash the ellipsoid into a unit sphere and measure the chord there.
	ox, oy, oz := (px-c.X)/c.R, (py-c.Y)/c.R, (pz-c.Z)/c.V
	ux, uy, uz := dx/c.R, dy/c.R, dz/c.V
	a := ux*ux + uy*uy + uz*uz
	b := 2 * (ox*ux + oy*uy + oz*uz)
	cc := ox*ox + oy*oy + oz*oz - 1
	disc := b*b - 4*a*cc
	if disc <= 0 {
		return 1
	}
	sq := math.Sqrt(disc)
	t0, t1 := (-b-sq)/(2*a), (-b+sq)/(2*a)
	if t1 <= 0 {
		return 1 // the crown is behind the walker, away from the sun
	}
	if t0 < 0 {
		t0 = 0
	}
	// Chord length in the unit sphere's units: 2 through the middle.
	chord := (t1 - t0) * math.Sqrt(a)
	return math.Exp(math.Log(c.Through) * chord / 2)
}
