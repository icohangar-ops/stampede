package report

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/icohangar-ops/stampede/internal/load"
	"github.com/icohangar-ops/stampede/internal/metrics"
)

func TestTemplateDegradedHeadlineAndCost(t *testing.T) {
	in := Input{
		URL:        "https://example.com/",
		PresetID:   "numberone",
		PresetName: "#1 Product of the Day",
		MaxRPS:     40,
		Summary: metrics.Summary{
			HasBreaking: true, BreakingRPS: 32, PeakRPS: 40, PeakP95: 1600, PeakError: 0.08,
		},
		Probe: load.ProbeResult{
			PageMS: 120,
			Assets: []load.Asset{
				{URL: "https://example.com/hero.png", Bytes: 180000, MS: 40, ContentType: "image/png"},
				{URL: "https://example.com/api/pricing", Bytes: 200, MS: 900},
			},
		},
	}
	rep := Template(in)
	if !strings.Contains(rep.Headline, "32") || !strings.Contains(strings.ToLower(rep.Headline), "degrades past") {
		t.Fatalf("headline %q", rep.Headline)
	}
	if rep.DegradesPastRPS != 32 || rep.Cost.USD <= 0 || !strings.Contains(strings.ToLower(rep.Cost.Note), "not an invoice") {
		t.Fatalf("%+v", rep)
	}
	blob := strings.ToLower(rep.Fixes[0].Title + rep.Fixes[1].Title + rep.Fixes[2].Title)
	if !strings.Contains(blob, "image") || !strings.Contains(blob, "slow") {
		t.Fatalf("fixes %+v", rep.Fixes)
	}
	// Breaking point is under the #1 cap but at/above a meaningful top5 line? 32 vs 0.75*40=30.
	// 32+0.5 is not < 30, so this is "degraded" (survives top 5), not "fails".
	if rep.Verdict != "degraded" {
		t.Fatalf("verdict %s", rep.Verdict)
	}
}

func TestTemplateFailsTop5AndLaunchReady(t *testing.T) {
	fail := Template(Input{
		PresetID: "top5", PresetName: "Top 5 of the Day", MaxRPS: 40,
		Summary: metrics.Summary{HasBreaking: true, BreakingRPS: 12, PeakRPS: 20, PeakP95: 2000, PeakError: 0.4},
	})
	if fail.Verdict != "fails" || !strings.Contains(fail.Headline, "Won't survive") {
		t.Fatalf("%+v", fail)
	}
	ok := Template(Input{
		PresetID: "top5", PresetName: "Top 5 of the Day", MaxRPS: 40,
		Summary: metrics.Summary{PeakRPS: 30, PeakP95: 80, PeakError: 0},
	})
	if ok.Verdict != "launch_ready" || !strings.Contains(ok.Headline, "Survives a Top 5") {
		t.Fatalf("%+v", ok)
	}
	if ok.Model != "template" {
		t.Fatal(ok.Model)
	}
}

func TestModelFallbackKeepsNumbers(t *testing.T) {
	b := Builder{LLM: fakeLLM{name: "gemini-2.5-flash", err: errors.New("down")}}
	in := Input{PresetID: "top5", MaxRPS: 40, Summary: metrics.Summary{HasBreaking: true, BreakingRPS: 18, PeakRPS: 20, PeakP95: 900, PeakError: 0.1}}
	rep := b.Build(context.Background(), in)
	if rep.Model != "template" || rep.DegradesPastRPS != 18 || rep.Notice == "" {
		t.Fatalf("%+v", rep)
	}
	b.LLM = fakeLLM{name: "gemini-2.5-flash", body: "sure {\"headline\":\"Survives a Top 5 launch, p95 degrades past ~18 req/s\",\"summary\":\"Custom.\",\"fixes\":[{\"title\":\"Cache\",\"detail\":\"Do it.\"}]} "}
	rep = b.Build(context.Background(), in)
	if rep.Model != "gemini-2.5-flash" || rep.Summary != "Custom." || rep.DegradesPastRPS != 18 {
		t.Fatalf("%+v", rep)
	}
	if rep.Cost.Note == "" || rep.Verdict != "fails" {
		t.Fatalf("numbers overwritten? %+v", rep)
	}
}

func TestEstimateMath(t *testing.T) {
	// peak 40 rps, p95 200ms -> concurrency 8 -> 1 instance.
	c := Estimate(40, 200)
	if c.Instances != 1 || c.Requests != 576000 {
		t.Fatalf("%+v", c)
	}
	// cpu-seconds = 1*7200 + 1*36000 = 43200
	// cpu 1.0368 + mem 0.054 + requests 0.2304 = 1.3212 -> 1.32
	if c.USD < 1.30 || c.USD > 1.34 {
		t.Fatalf("usd %v", c.USD)
	}
}

type fakeLLM struct {
	name string
	body string
	err  error
}

func (f fakeLLM) Name() string { return f.name }
func (f fakeLLM) Complete(context.Context, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.body, nil
}
