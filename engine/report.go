package engine

import (
	"fmt"
	"strings"
)

func pct(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

func clock(h float64) string {
	m := int(h*60 + 0.5)
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// Text is the command line's plain-text report.
func (r *Result) Text() string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	s, p := r.Scene, r.Plan
	w("SHADEGROVE  %s\n", s.Name)
	w("%s, %s to %s, up to %.0f °C. %d footpath cells of %.2g m, %d moments with the sun up.\n",
		s.Date, clock(s.FromHour), clock(s.ToHour), s.MaxTemp, len(r.CellX), r.CellSize, len(r.Moments))
	w("\n")
	w("Plant %d trees (best first; each gain counts only what earlier trees don't already shade):\n", len(p.Picks))
	for _, pk := range p.Picks {
		sp, _ := SpeciesByID(pk.Species)
		w("  %2d. %-6s %-14s at (%6.1f, %6.1f)  removes %5.1f%% of the street's hot sun\n", pk.Order, pk.Site, sp.Name, pk.X, pk.Y, pk.GainShare*100)
	}
	w("\n")
	w("After %d years these trees remove %s of the heat-weighted sun on the footpaths.\n", s.Year, pct(p.Reduction))
	w("No plan of %d trees can remove more than %s, so this one is at least %s of the best possible.\n",
		s.Budget, pct(share(p.UpperBound, p.Before)), pct(p.Guarantee))
	w("(%d candidate site-and-species pairs; lazy greedy needed %d gain evaluations.)\n", p.Candidates, p.Evaluations)
	w("\n")
	w("As they grow:")
	for _, g := range r.Growth {
		w("  year %d: %s", g.Year, pct(g.Reduction))
	}
	w("\n")
	w("Winter (%s): the same trees take %s of the winter sun on the footpaths.\n", r.Seasons.WinterDate, pct(r.Seasons.WinterLost))
	for _, n := range r.Notes {
		w("Note: %s\n", n)
	}
	w("\nIndicative species figures; check your council's street-tree guide and what's underground before planting.\n")
	return b.String()
}
