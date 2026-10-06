package preset

import "testing"

func TestPresetPeaksAndRamp(t *testing.T) {
	top := Top5()
	one := NumberOne()
	if top.Name != "Top 5 of the Day" || one.Name != "#1 Product of the Day" {
		t.Fatalf("names: %s %s", top.Name, one.Name)
	}
	if len(top.Assumptions) == 0 || len(one.Assumptions) == 0 {
		t.Fatal("assumptions missing")
	}
	var topPeak, onePeak float64
	for i := 0; i <= 100; i++ {
		f := float64(i) / 100
		if v := top.Intensity(f); v > topPeak {
			topPeak = v
		}
		if v := one.Intensity(f); v > onePeak {
			onePeak = v
		}
	}
	if topPeak < 0.74 || topPeak > 0.76 {
		t.Fatalf("top5 peak %v", topPeak)
	}
	if onePeak < 0.99 {
		t.Fatalf("number one peak %v", onePeak)
	}
	if onePeak <= topPeak {
		t.Fatalf("number one should peak higher: %v vs %v", onePeak, topPeak)
	}
	// Steeper: number one reaches 0.9 earlier than top5 reaches its own peak.
	oneAt := -1
	for i := 0; i <= 100; i++ {
		if one.Intensity(float64(i)/100) >= 0.9 {
			oneAt = i
			break
		}
	}
	topAt := -1
	for i := 0; i <= 100; i++ {
		if top.Intensity(float64(i)/100) >= 0.74 {
			topAt = i
			break
		}
	}
	if oneAt < 0 || topAt < 0 || oneAt >= topAt {
		t.Fatalf("expected a steeper #1 ramp, oneAt=%d topAt=%d", oneAt, topAt)
	}

	secs := 45
	if got := top.Workers(Fraction(0, secs)); got != StartWorkers {
		t.Fatalf("start workers %d", got)
	}
	if got := top.Workers(Fraction(secs-1, secs)); got != EndWorkers {
		t.Fatalf("end workers %d", got)
	}
	if got := one.Workers(Fraction(secs-1, secs)); got != EndWorkers {
		t.Fatalf("number one end workers %d", got)
	}
}

func TestByID(t *testing.T) {
	if _, ok := ByID("nope"); ok {
		t.Fatal("unknown preset")
	}
	p, ok := ByID("top5")
	if !ok || p.ID != "top5" {
		t.Fatal(p, ok)
	}
}
