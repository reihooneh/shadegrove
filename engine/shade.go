package engine

import "math"

// EyeHeight is where on a walker the sun is judged to fall (roughly the shoulders), m.
const EyeHeight = 1.5

// Cell is a small square of footpath.
type Cell struct {
	X, Y   float64 // centre
	Weight float64 // footfall x area
	Walk   int
}

// Step is one moment of the day.
type Step struct {
	Hour   float64
	Sun    Sun
	Dir    [3]float64 // unit vector towards the sun, in the scene's frame
	Weight float64    // how much sun at this moment matters: direct beam x heat of the hour x duration
	Heat   float64    // air temperature estimate, °C
}

// Grid is the footpath cut into cells, and the day cut into steps.
type Grid struct {
	Cells    []Cell
	CellSize float64
	Steps    []Step
}

// MakeGrid cuts the walks into cells (as fine as 1 m, coarser if the walks are big) and the day
// into steps with the sun up.
func MakeGrid(s *Scene) *Grid {
	st := steps(s)
	limit := MaxCells
	if len(st) > 0 {
		limit = min(MaxCells, MaxCellSteps/len(st))
	}
	area := 0.0
	for _, w := range s.Walks {
		area += (w.X1 - w.X0) * (w.Y1 - w.Y0)
	}
	size := math.Max(1, math.Ceil(math.Sqrt(area/float64(limit))*4)/4)
	for cellsFor(s, size) > limit {
		size += 0.25
	}
	g := &Grid{CellSize: size}
	for wi, w := range s.Walks {
		nx := int(math.Max(1, math.Round((w.X1-w.X0)/size)))
		ny := int(math.Max(1, math.Round((w.Y1-w.Y0)/size)))
		dx, dy := (w.X1-w.X0)/float64(nx), (w.Y1-w.Y0)/float64(ny)
		for i := 0; i < nx; i++ {
			for j := 0; j < ny; j++ {
				g.Cells = append(g.Cells, Cell{X: w.X0 + (float64(i)+0.5)*dx, Y: w.Y0 + (float64(j)+0.5)*dy, Weight: w.Footfall * dx * dy, Walk: wi})
			}
		}
	}
	g.Steps = st
	return g
}

func cellsFor(s *Scene, size float64) int {
	n := 0
	for _, w := range s.Walks {
		n += int(math.Max(1, math.Round((w.X1-w.X0)/size))) * int(math.Max(1, math.Round((w.Y1-w.Y0)/size)))
	}
	return n
}

// Temperature through a hot day: coolest around 5 am, hottest around 3 pm (a cosine between).
func temperature(hour, maxTemp float64) float64 {
	minTemp := maxTemp - 12
	var phase float64
	switch {
	case hour >= 5 && hour <= 15:
		phase = (hour - 5) / 10 // warming
		return minTemp + (maxTemp-minTemp)*(1-math.Cos(math.Pi*phase))/2
	default:
		h := hour
		if h < 5 {
			h += 24
		}
		phase = (h - 15) / 14 // cooling
		return maxTemp - (maxTemp-minTemp)*(1-math.Cos(math.Pi*phase))/2
	}
}

func steps(s *Scene) []Step {
	y, m, d := s.Day()
	dt := float64(s.StepMin) / 60
	var out []Step
	for h := s.FromHour + dt/2; h < s.ToHour; h += dt {
		sun := SunAt(s.Latitude, s.Longitude, s.UTCOffset, y, m, d, h)
		if sun.Elevation <= 2 {
			continue
		}
		rel := (sun.Azimuth - s.Bearing) * deg
		ce := math.Cos(sun.Elevation * deg)
		dir := [3]float64{math.Sin(rel) * ce, math.Cos(rel) * ce, math.Sin(sun.Elevation * deg)}
		t := temperature(h, s.MaxTemp)
		// Sun matters more the hotter it is: weight by how far the air is above a comfortable 24 °C,
		// with a floor so morning sun still counts a little.
		heat := math.Max(0.15, (t-24)/math.Max(1, s.MaxTemp-24))
		out = append(out, Step{Hour: h, Sun: sun, Dir: dir, Weight: DirectBeam(sun.Elevation) / 1000 * heat * dt, Heat: t})
	}
	return out
}

// rayHitsBox reports whether a ray from p towards d (d pointing up) passes through the box.
// The slab method: clip the ray against each pair of parallel faces.
func rayHitsBox(px, py, pz float64, d [3]float64, b Box) bool {
	lo := [3]float64{b.X, b.Y, 0}
	hi := [3]float64{b.X + b.W, b.Y + b.D, b.H}
	p := [3]float64{px, py, pz}
	tmin, tmax := 1e-6, math.Inf(1)
	for k := 0; k < 3; k++ {
		if math.Abs(d[k]) < 1e-12 {
			if p[k] < lo[k] || p[k] > hi[k] {
				return false
			}
			continue
		}
		t1, t2 := (lo[k]-p[k])/d[k], (hi[k]-p[k])/d[k]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tmin = math.Max(tmin, t1)
		tmax = math.Min(tmax, t2)
		if tmin > tmax {
			return false
		}
	}
	return true
}

// Exposure is how much heat-weighted sun reaches each cell at each step.
type Exposure struct {
	Grid   *Grid
	Value  []float64 // per cell x step: weight of that moment for that cell, before any shade
	Sunlit []float64 // per cell x step: fraction of sun getting through buildings and existing trees
}

// Index returns the flat index of a cell at a step.
func (e *Exposure) Index(cell, step int) int { return cell*len(e.Grid.Steps) + step }

// Total is the heat-weighted sun reaching the walks, given what fraction gets through at each moment.
func (e *Exposure) Total(through []float64) float64 {
	t := 0.0
	for i, v := range e.Value {
		t += v * through[i]
	}
	return t
}

// Measure works out the shade cast by buildings and existing trees at every cell and step.
func Measure(s *Scene, g *Grid, winter bool) *Exposure {
	n := len(g.Cells) * len(g.Steps)
	e := &Exposure{Grid: g, Value: make([]float64, n), Sunlit: make([]float64, n)}
	var crowns []Canopy
	for _, t := range s.Existing {
		sp, _ := SpeciesByID(t.Species)
		crowns = append(crowns, sp.CanopyAt(t.X, t.Y, t.Age, winter && sp.Deciduous))
	}
	for c, cell := range g.Cells {
		for k, st := range g.Steps {
			i := e.Index(c, k)
			e.Value[i] = cell.Weight * st.Weight
			lit := 1.0
			for _, b := range s.Buildings {
				if rayHitsBox(cell.X, cell.Y, EyeHeight, st.Dir, b) {
					lit = 0
					break
				}
			}
			for _, cr := range crowns {
				if lit == 0 {
					break
				}
				lit *= cr.Transmit(cell.X, cell.Y, EyeHeight, st.Dir[0], st.Dir[1], st.Dir[2])
			}
			e.Sunlit[i] = lit
		}
	}
	return e
}
