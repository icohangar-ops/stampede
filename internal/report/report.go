// Package report turns measured samples into a readiness verdict.
// Numbers always come from the samples. A model may only rewrite wording.
package report

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/icohangar-ops/stampede/internal/load"
	"github.com/icohangar-ops/stampede/internal/metrics"
)

const costNote = "Approximation of Cloud Run list price for request-based billing in a tier-1 region (about 1 vCPU and 512 MiB, concurrency 80). Two hours at the observed peak and ten hours at 20% of peak. Free tier and taxes are not subtracted. This is not an invoice."

// List prices, request-based billing, published order of magnitude.
const (
	cpuPerVCPUSecond = 0.000024
	memPerGiBSecond  = 0.0000025
	usdPerMillionReq = 0.40
	assumeVCPU       = 1.0
	assumeMemGiB     = 0.5
	assumeConc       = 80.0
)

// Fix is one concrete change a maker can make.
type Fix struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Cost is the launch-day Cloud Run estimate.
type Cost struct {
	USD       float64 `json:"usd"`
	Instances int     `json:"instances"`
	Requests  int64   `json:"requests"`
	Note      string  `json:"note"`
}

// Report is what the UI and the badge read.
type Report struct {
	Verdict         string  `json:"verdict"`
	Headline        string  `json:"headline"`
	Summary         string  `json:"summary"`
	Fixes           []Fix   `json:"fixes"`
	DegradesPastRPS float64 `json:"degrades_past_rps"`
	HasBreaking     bool    `json:"has_breaking"`
	Cost            Cost    `json:"cost"`
	Model           string  `json:"model"`
	Notice          string  `json:"notice,omitempty"`
}

// Input is everything the report is allowed to see.
type Input struct {
	URL        string           `json:"url"`
	PresetID   string           `json:"preset_id"`
	PresetName string           `json:"preset_name"`
	MaxRPS     float64          `json:"max_rps"`
	Summary    metrics.Summary  `json:"summary"`
	Probe      load.ProbeResult `json:"probe"`
}

// LLM rewrites prose. Implementations must not be trusted for numbers.
type LLM interface {
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

// Builder produces reports.
type Builder struct {
	LLM LLM
}

// Build returns a template report, optionally reworded by a model.
// Measured rates, the breaking point, the verdict, and the cost survive a model response.
func (b Builder) Build(ctx context.Context, in Input) Report {
	base := Template(in)
	if b.LLM == nil {
		base.Model = "template"
		return base
	}
	raw, err := b.LLM.Complete(ctx, prompt(in, base))
	if err != nil {
		base.Model = "template"
		base.Notice = "The model was unavailable, so this is the deterministic report."
		return base
	}
	var parsed struct {
		Headline string `json:"headline"`
		Summary  string `json:"summary"`
		Fixes    []Fix  `json:"fixes"`
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &parsed); err != nil || strings.TrimSpace(parsed.Headline) == "" {
		base.Model = "template"
		base.Notice = "The model reply could not be used, so this is the deterministic report."
		return base
	}
	base.Headline = parsed.Headline
	if strings.TrimSpace(parsed.Summary) != "" {
		base.Summary = parsed.Summary
	}
	if len(parsed.Fixes) > 0 {
		base.Fixes = parsed.Fixes
	}
	base.Model = b.LLM.Name()
	return base
}

// Template is the deterministic report. It does not call a model.
func Template(in Input) Report {
	s := in.Summary
	rep := Report{
		HasBreaking:     s.HasBreaking,
		DegradesPastRPS: s.BreakingRPS,
		Cost:            Estimate(s.PeakRPS, s.PeakP95),
		Model:           "template",
	}
	top5Line := 0.75 * in.MaxRPS
	switch {
	case !s.HasBreaking && s.PeakError < 0.02 && s.PeakP95 < 800:
		rep.Verdict = "launch_ready"
		if in.PresetID == "numberone" {
			rep.Headline = "Survives a #1 Product of the Day launch"
		} else {
			rep.Headline = "Survives a Top 5 launch"
		}
		rep.Summary = fmt.Sprintf("Across this %s shape, peak was %.0f req/s with p95 %.0f ms and %.1f%% errors. Nothing crossed the breaking line.",
			or(in.PresetName, "preset"), s.PeakRPS, s.PeakP95, s.PeakError*100)
	case s.HasBreaking && s.BreakingRPS+0.5 < top5Line:
		rep.Verdict = "fails"
		rep.Headline = fmt.Sprintf("Won't survive a Top 5 launch, %s ~%.0f req/s", degradeWord(s), s.BreakingRPS)
		rep.Summary = fmt.Sprintf("The run broke at about %.0f req/s, which is under the Top 5 plateau. Peak p95 was %.0f ms and the worst error rate was %.1f%%.",
			s.BreakingRPS, s.PeakP95, s.PeakError*100)
	default:
		rep.Verdict = "degraded"
		if s.HasBreaking {
			rep.Headline = fmt.Sprintf("Survives a Top 5 launch, %s ~%.0f req/s", degradeWord(s), s.BreakingRPS)
			rep.Summary = fmt.Sprintf("The curve stayed healthy into the Top 5 band, then p95 or errors gave way around %.0f req/s. Worst p95 was %.0f ms.",
				s.BreakingRPS, s.PeakP95)
		} else {
			rep.Verdict = "degraded"
			rep.Headline = fmt.Sprintf("Degraded through the run, worst p95 %.0f ms", s.PeakP95)
			rep.Summary = "No single second crossed the breaking thresholds, but latency or errors were high enough that launch morning would feel rough."
		}
	}
	rep.Fixes = fixes(in)
	if len(rep.Fixes) == 0 {
		rep.Fixes = []Fix{{
			Title:  "Hold one warm instance for launch hour",
			Detail: "Set Cloud Run min instances to 1 for the morning of the launch so the first visitors are not the ones who pay for a cold start.",
		}}
	}
	return rep
}

func degradeWord(s metrics.Summary) string {
	if s.BreakReason == "error" || (s.BreakReason == "" && s.PeakError > metrics.ErrorBreak && s.PeakP95 <= metrics.P95BreakMS) {
		return "errors climb past"
	}
	return "p95 degrades past"
}

func fixes(in Input) []Fix {
	var out []Fix
	s := in.Summary
	var heavy load.Asset
	var slow load.Asset
	for _, a := range in.Probe.Assets {
		if isImage(a) && a.Bytes > heavy.Bytes {
			heavy = a
		}
		if a.MS > slow.MS {
			slow = a
		}
	}
	if heavy.Bytes >= 100_000 {
		out = append(out, Fix{
			Title:  "Cut the image weight",
			Detail: fmt.Sprintf("%s is %d KB. Resize and compress it, and serve it from a CDN, before launch morning.", pathOf(heavy.URL), heavy.Bytes/1024),
		})
	}
	if slow.MS >= 700 || in.Probe.PageMS >= 700 {
		target := in.URL
		ms := in.Probe.PageMS
		if slow.MS >= in.Probe.PageMS && slow.URL != "" {
			target = slow.URL
			ms = slow.MS
		}
		out = append(out, Fix{
			Title:  "Speed up the slow endpoint",
			Detail: fmt.Sprintf("%s took about %.0f ms on a single quiet fetch. Cache it or move the work off the request path.", pathOf(target), ms),
		})
	}
	if s.PeakP95 >= 400 || s.HasBreaking {
		out = append(out, Fix{
			Title:  "Cache and put a CDN in front",
			Detail: "p95 climbed under the ramp. Cache the HTML if it can be public, and serve static assets from a CDN so Cloud Run only sees the dynamic calls.",
		})
	}
	if s.PeakError > 0.01 {
		out = append(out, Fix{
			Title:  "Raise concurrency headroom",
			Detail: "Errors showed up before the safety cap. On Cloud Run, raise max instances for launch morning and check that the container's concurrency matches what the process can actually serve.",
		})
	}
	out = append(out, Fix{
		Title:  "Hold one warm instance for launch hour",
		Detail: "Set Cloud Run min instances to 1 until the front page cools off. Scale-to-zero is right every other day. It is the wrong default at 12:01 AM Pacific.",
	})
	return out
}

// Estimate prices a launch day that looks like the measured peak.
func Estimate(peakRPS, peakP95MS float64) Cost {
	if peakRPS < 0 {
		peakRPS = 0
	}
	if peakP95MS < 0 {
		peakP95MS = 0
	}
	conc := peakRPS * (peakP95MS / 1000)
	instances := int(math.Ceil(conc / assumeConc))
	if instances < 1 && peakRPS > 0 {
		instances = 1
	}
	tailInstances := int(math.Ceil(float64(instances) * 0.2))
	if instances > 0 && tailInstances < 1 {
		tailInstances = 1
	}
	const peakSec = 2 * 3600
	const tailSec = 10 * 3600
	cpuSeconds := float64(instances)*peakSec + float64(tailInstances)*tailSec
	requests := int64(math.Round(peakRPS*peakSec + peakRPS*0.2*tailSec))
	usd := cpuSeconds*assumeVCPU*cpuPerVCPUSecond + cpuSeconds*assumeMemGiB*memPerGiBSecond + float64(requests)/1e6*usdPerMillionReq
	return Cost{
		USD:       math.Round(usd*100) / 100,
		Instances: instances,
		Requests:  requests,
		Note:      costNote,
	}
}

func prompt(in Input, base Report) string {
	blob, _ := json.Marshal(struct {
		URL         string  `json:"url"`
		Preset      string  `json:"preset"`
		PeakRPS     float64 `json:"peak_rps"`
		PeakP95     float64 `json:"peak_p95_ms"`
		PeakError   float64 `json:"peak_error_rate"`
		Breaking    float64 `json:"breaking_rps"`
		HasBreaking bool    `json:"has_breaking"`
		Verdict     string  `json:"verdict"`
		Headline    string  `json:"template_headline"`
		ProbeMS     float64 `json:"probe_page_ms"`
	}{
		URL: in.URL, Preset: in.PresetName, PeakRPS: in.Summary.PeakRPS, PeakP95: in.Summary.PeakP95,
		PeakError: in.Summary.PeakError, Breaking: in.Summary.BreakingRPS, HasBreaking: in.Summary.HasBreaking,
		Verdict: base.Verdict, Headline: base.Headline, ProbeMS: in.Probe.PageMS,
	})
	return `You are writing a launch-readiness note for an indie maker. Use only the numbers in the JSON. Do not invent traffic, customers, or prices.
Return JSON with keys headline, summary, fixes (array of {title, detail}).
Keep the headline under 120 characters. If has_breaking is true, the headline must mention the breaking rate.
Give two or three concrete fixes about caching, CDN, image weight, slow endpoints, or Cloud Run instance settings.
JSON:
` + string(blob)
}

func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			return raw[i : j+1]
		}
	}
	return raw
}

func isImage(a load.Asset) bool {
	if strings.HasPrefix(a.ContentType, "image/") {
		return true
	}
	p := strings.ToLower(a.URL)
	return strings.Contains(p, ".png") || strings.Contains(p, ".jpg") || strings.Contains(p, ".jpeg") || strings.Contains(p, ".webp") || strings.Contains(p, ".gif")
}

func pathOf(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j:]
		}
	}
	if raw == "" {
		return "the page"
	}
	return raw
}

func or(a, b string) string {
	if a == "" {
		return b
	}
	return a
}
