// Package preset holds the Product Hunt launch-day shapes Stampede can play.
// Magnitudes are scaled to the safety cap. The shapes are planning assumptions,
// not an official Product Hunt feed. See the README for the visitor bands.
package preset

import (
	"math"
	"time"
)

const (
	DefaultDuration = 45 * time.Second
	StartWorkers    = 10
	EndWorkers      = 50
)

// Preset is a named traffic curve.
type Preset struct {
	ID           string
	Name         string
	Blurb        string
	Assumptions  []string
	Duration     time.Duration
	StartWorkers int
	EndWorkers   int
	curve        func(float64) float64
	peak         float64
}

// PeakFraction is the highest share of the safety cap this curve reaches.
func (p Preset) PeakFraction() float64 { return p.peak }

// Intensity returns the cap fraction at frac, where frac is 0 at the first
// second and 1 at the last second.
func (p Preset) Intensity(frac float64) float64 {
	frac = clamp01(frac)
	v := p.curve(frac)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Workers interpolates parallel workers from the start count to the end count.
func (p Preset) Workers(frac float64) int {
	frac = clamp01(frac)
	w := float64(p.StartWorkers) + frac*float64(p.EndWorkers-p.StartWorkers)
	n := int(math.Round(w))
	if n < 1 {
		return 1
	}
	return n
}

// RPS is the curve intensity times the safety cap.
func (p Preset) RPS(frac, maxRPS float64) float64 {
	return p.Intensity(frac) * maxRPS
}

// Fraction maps a second index onto 0..1 inclusive of both ends.
func Fraction(sec, total int) float64 {
	if total <= 1 {
		return 1
	}
	if sec < 0 {
		sec = 0
	}
	if sec > total-1 {
		sec = total - 1
	}
	return float64(sec) / float64(total-1)
}

// All returns the presets in display order.
func All() []Preset {
	return []Preset{Top5(), NumberOne()}
}

// ByID looks up a preset.
func ByID(id string) (Preset, bool) {
	for _, p := range All() {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Top5 is a climb, a plateau at 75% of the cap, then an ease-off.
func Top5() Preset {
	return Preset{
		ID:           "top5",
		Name:         "Top 5 of the Day",
		Blurb:        "A front-page morning: steady climb, a plateau, then the crowd thins.",
		Assumptions:  append([]string(nil), sharedAssumptions...),
		Duration:     DefaultDuration,
		StartWorkers: StartWorkers,
		EndWorkers:   EndWorkers,
		curve:        top5Curve,
		peak:         0.75,
	}
}

// NumberOne is a steep spike that holds the full safety cap, then decays.
func NumberOne() Preset {
	return Preset{
		ID:           "numberone",
		Name:         "#1 Product of the Day",
		Blurb:        "The sharp first-hour spike. Steeper than a Top 5, and it sits on the cap.",
		Assumptions:  append([]string{"Plan on roughly 6,000–15,000 launch-day uniques, with about a quarter of them in the first two hours."}, sharedAssumptions...),
		Duration:     DefaultDuration,
		StartWorkers: StartWorkers,
		EndWorkers:   EndWorkers,
		curve:        numberOneCurve,
		peak:         1,
	}
}

var sharedAssumptions = []string{
	"These are planning shapes, not an official Product Hunt traffic feed.",
	"A typical maker page is about 10 HTTP requests per visitor (document plus assets).",
	"Stampede compresses the launch morning into a short run, 45 seconds by default locally and 30 seconds on the Vercel demo, never more than 3 minutes.",
	"Absolute rates are scaled to the safety cap (default 40 requests/second and 50 workers) so the shape is useful and the tool cannot be aimed as a flood.",
	"Top 5 of the Day assumes roughly 2,000–4,000 launch-day uniques. The plateau is 75% of the cap.",
	"Workers ramp from 10 to 50 across the run. Locally those are goroutines. On Vercel they run in-process inside one function. The optional Cloud Run job splits them across parallel tasks.",
}

func top5Curve(f float64) float64 {
	switch {
	case f < 0.20:
		return lerp(0.22, 0.45, f/0.20)
	case f < 0.55:
		return lerp(0.45, 0.75, (f-0.20)/0.35)
	case f < 0.80:
		return 0.75
	default:
		return lerp(0.75, 0.48, (f-0.80)/0.20)
	}
}

func numberOneCurve(f float64) float64 {
	switch {
	case f < 0.08:
		return lerp(0.30, 0.55, f/0.08)
	case f < 0.28:
		return lerp(0.55, 1.00, (f-0.08)/0.20)
	case f < 0.68:
		return 1
	default:
		return lerp(1, 0.62, (f-0.68)/0.32)
	}
}

func lerp(a, b, t float64) float64 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return a + (b-a)*t
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
