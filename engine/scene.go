package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"time"
)

// Limits on a scene. They keep every run to a few seconds in a browser.
const (
	MaxSceneBytes = 256 * 1024
	MaxBuildings  = 200
	MaxWalks      = 60
	MaxSites      = 80
	MaxExisting   = 100
	MaxCells      = 6000
	MaxExtent     = 600.0 // metres from the origin
)

// Box is a building: an axis-aligned block in the scene's frame, in metres.
type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"` // along x
	D float64 `json:"d"` // along y
	H float64 `json:"h"`
}

// Walk is a stretch of footpath (or a plaza) where people walk, with how busy it is.
type Walk struct {
	X0       float64 `json:"x0"`
	Y0       float64 `json:"y0"`
	X1       float64 `json:"x1"`
	Y1       float64 `json:"y1"`
	Footfall float64 `json:"footfall"` // relative: 1 = ordinary, 3 = three times as busy
	Name     string  `json:"name,omitempty"`
}

// Site is a place a tree could be planted.
type Site struct {
	ID        string  `json:"id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	MaxHeight float64 `json:"maxHeight,omitempty"` // e.g. under power lines; 0 = no limit
}

// Tree is a tree already growing.
type Tree struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Species string  `json:"species"`
	Age     float64 `json:"age"` // years since planting
}

// Scene is everything about a street the planner needs.
type Scene struct {
	Name       string   `json:"name"`
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
	UTCOffset  float64  `json:"utcOffset"` // hours, the clock in use on the day (e.g. 11 for Sydney in summer)
	Date       string   `json:"date"`      // YYYY-MM-DD: the hot day to plan for
	Bearing    float64  `json:"bearing"`   // compass bearing of the scene's +y axis (0 = north)
	FromHour   float64  `json:"fromHour"`
	ToHour     float64  `json:"toHour"`
	StepMin    int      `json:"stepMinutes"`
	MaxTemp    float64  `json:"maxTemp"` // °C on the day
	Buildings  []Box    `json:"buildings"`
	Walks      []Walk   `json:"walks"`
	Sites      []Site   `json:"sites"`
	Existing   []Tree   `json:"existing"`
	Species    []string `json:"species"`    // species allowed (empty = all)
	Budget     int      `json:"budget"`     // trees to plant
	Year       int      `json:"targetYear"` // years after planting to optimise for
	MinSpacing float64  `json:"minSpacing"` // metres between new trees (and from existing ones)
	MaxShare   float64  `json:"maxShare"`   // at most this share of new trees from one species (a pest can't take the whole street)

	date time.Time
}

// InputError is a problem with the scene. Its message never repeats the input.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func bad(format string, args ...any) error { return &InputError{Msg: fmt.Sprintf(format, args...)} }

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 '.,&()/-]{0,59}$`)
	idRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,15}$`)
)

// ParseScene reads and checks a scene from JSON. Unknown fields, trailing data, oversize input and
// out-of-range values are all refused.
func ParseScene(r io.Reader) (*Scene, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxSceneBytes+1))
	if err != nil {
		return nil, bad("the scene could not be read")
	}
	if len(data) > MaxSceneBytes {
		return nil, bad("the scene is larger than 256 KB")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s Scene
	if err := dec.Decode(&s); err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syn):
			return nil, bad("the scene is not valid JSON (near byte %d)", syn.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF):
			return nil, bad("the scene is not valid JSON (it ends early)")
		case errors.As(err, &typ):
			return nil, bad("the field %q has the wrong type", typ.Field)
		default:
			return nil, bad("the scene has a field Shadegrove doesn't know, or isn't a JSON object")
		}
	}
	if dec.More() {
		return nil, bad("the scene has extra data after the JSON object")
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

func within(v, lo, hi float64) bool { return v >= lo && v <= hi }

// Validate checks every field and fills in defaults.
func (s *Scene) Validate() error {
	if s.Name == "" {
		s.Name = "Untitled street"
	}
	if !nameRe.MatchString(s.Name) {
		return bad("the name uses letters, digits, spaces and ' . , & ( ) / - (up to 60)")
	}
	if !within(s.Latitude, -66, 66) || !within(s.Longitude, -180, 180) {
		return bad("latitude must be within 66° of the equator and longitude within 180°")
	}
	if !within(s.UTCOffset, -12, 14) || !within(s.Bearing, 0, 360) {
		return bad("utcOffset is -12 to 14 hours and bearing 0 to 360°")
	}
	d, err := time.Parse("2006-01-02", s.Date)
	if err != nil || d.Year() < 1950 || d.Year() > 2150 {
		return bad("date must be a real date written YYYY-MM-DD (1950 to 2150)")
	}
	s.date = d
	if s.FromHour == 0 && s.ToHour == 0 {
		s.FromHour, s.ToHour = 9, 18
	}
	if !within(s.FromHour, 4, 22) || !within(s.ToHour, s.FromHour+1, 23) {
		return bad("the hours run from 4 to 23, at least an hour apart")
	}
	if s.StepMin == 0 {
		s.StepMin = 15
	}
	if s.StepMin < 10 || s.StepMin > 60 {
		return bad("stepMinutes is 10 to 60")
	}
	if s.MaxTemp == 0 {
		s.MaxTemp = 38
	}
	if !within(s.MaxTemp, 20, 55) {
		return bad("maxTemp is 20 to 55 °C")
	}
	if s.Budget == 0 {
		s.Budget = 10
	}
	if s.Budget < 1 || s.Budget > MaxSites {
		return bad("budget is 1 to %d trees", MaxSites)
	}
	if s.Year == 0 {
		s.Year = 10
	}
	if s.Year < 1 || s.Year > 40 {
		return bad("targetYear is 1 to 40 years")
	}
	if s.MinSpacing == 0 {
		s.MinSpacing = 7
	}
	if !within(s.MinSpacing, 2, 30) {
		return bad("minSpacing is 2 to 30 m")
	}
	if s.MaxShare == 0 {
		s.MaxShare = 0.5
	}
	if !within(s.MaxShare, 0.1, 1) {
		return bad("maxShare is 0.1 to 1")
	}
	if !finite(s.Latitude, s.Longitude, s.UTCOffset, s.Bearing, s.FromHour, s.ToHour, s.MaxTemp, s.MinSpacing, s.MaxShare) {
		return bad("numbers must be finite")
	}

	if len(s.Buildings) > MaxBuildings || len(s.Walks) == 0 || len(s.Walks) > MaxWalks ||
		len(s.Sites) == 0 || len(s.Sites) > MaxSites || len(s.Existing) > MaxExisting {
		return bad("a scene has up to %d buildings, 1 to %d walks, 1 to %d sites and up to %d existing trees",
			MaxBuildings, MaxWalks, MaxSites, MaxExisting)
	}
	coord := func(vs ...float64) bool {
		for _, v := range vs {
			if !finite(v) || math.Abs(v) > MaxExtent {
				return false
			}
		}
		return true
	}
	for i, b := range s.Buildings {
		if !coord(b.X, b.Y, b.X+b.W, b.Y+b.D) || !within(b.W, 0.5, 300) || !within(b.D, 0.5, 300) || !within(b.H, 1, 300) {
			return bad("building %d: sizes are 0.5 to 300 m and it must lie within %g m of the origin", i+1, MaxExtent)
		}
	}
	area := 0.0
	for i := range s.Walks {
		w := &s.Walks[i]
		if !coord(w.X0, w.Y0, w.X1, w.Y1) || w.X1-w.X0 < 0.5 || w.Y1-w.Y0 < 0.5 {
			return bad("walk %d: x0 must be less than x1 and y0 less than y1, within %g m of the origin", i+1, MaxExtent)
		}
		if w.Footfall == 0 {
			w.Footfall = 1
		}
		if !within(w.Footfall, 0.1, 100) || !finite(w.Footfall) {
			return bad("walk %d: footfall is 0.1 to 100", i+1)
		}
		if w.Name != "" && !nameRe.MatchString(w.Name) {
			return bad("walk %d: names use letters, digits, spaces and ' . , & ( ) / -", i+1)
		}
		area += (w.X1 - w.X0) * (w.Y1 - w.Y0)
	}
	if area > 40000 {
		return bad("the walks cover more than 40,000 m²; split the street into smaller scenes")
	}
	ids := map[string]bool{}
	for i, t := range s.Sites {
		if !idRe.MatchString(t.ID) {
			return bad("site %d: ids are letters, digits, . _ - (up to 16)", i+1)
		}
		if ids[t.ID] {
			return bad("site %d: the id is used twice", i+1)
		}
		ids[t.ID] = true
		if !coord(t.X, t.Y) || !within(t.MaxHeight, 0, 60) || !finite(t.MaxHeight) {
			return bad("site %d: must lie within %g m of the origin; maxHeight is 0 to 60 m", i+1, MaxExtent)
		}
	}
	for i, t := range s.Existing {
		if _, ok := SpeciesByID(t.Species); !ok {
			return bad("existing tree %d: unknown species", i+1)
		}
		if !coord(t.X, t.Y) || !within(t.Age, 0, 150) || !finite(t.Age) {
			return bad("existing tree %d: must lie within %g m of the origin, aged 0 to 150 years", i+1, MaxExtent)
		}
	}
	for _, id := range s.Species {
		if _, ok := SpeciesByID(id); !ok {
			return bad("unknown species in the allowed list")
		}
	}
	return nil
}

// Day returns the scene's date parts.
func (s *Scene) Day() (int, int, int) { return s.date.Year(), int(s.date.Month()), s.date.Day() }

// Winter returns the same scene half a year later: the season when shade is least welcome.
func (s *Scene) Winter() *Scene {
	w := *s
	w.date = s.date.AddDate(0, 6, 0)
	w.Date = w.date.Format("2006-01-02")
	return &w
}

// AllowedSpecies lists the species the planner may choose from.
func (s *Scene) AllowedSpecies() []Species {
	if len(s.Species) == 0 {
		return Catalogue
	}
	var out []Species
	for _, id := range s.Species {
		sp, _ := SpeciesByID(id)
		out = append(out, sp)
	}
	return out
}
