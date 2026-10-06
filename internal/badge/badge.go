// Package badge draws the shareable launch mark.
package badge

import (
	"fmt"
	"html"
	"strings"
)

// SVG returns a dark badge. Only a completed run with a launch_ready verdict
// is allowed to say Launch-ready.
func SVG(status, verdict string, p95 float64, hasBreaking bool) string {
	label := "In progress"
	tone := "#8a8178"
	if status == "failed" {
		label = "Run failed"
		tone = "#ff5d73"
	} else if status == "cancelled" {
		label = "Cancelled"
		tone = "#ffb020"
	} else if status == "completed" {
		if verdict == "launch_ready" && !hasBreaking {
			label = "Launch-ready"
			tone = "#c6f135"
		} else {
			label = "Needs work"
			tone = "#ff4d00"
		}
	}
	sub := "Stampede"
	if status == "completed" {
		if hasBreaking {
			sub = "Broke under the ramp"
		} else if p95 > 0 {
			sub = fmt.Sprintf("p95 %.0f ms", p95)
		} else {
			sub = "No breaking point"
		}
	}
	return strings.TrimSpace(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="360" height="64" viewBox="0 0 360 64" role="img" aria-label="%s">
  <rect width="360" height="64" rx="10" fill="#14110e"/>
  <rect x="1" y="1" width="358" height="62" rx="9" fill="none" stroke="%s" stroke-width="2"/>
  <text x="16" y="28" fill="%s" font-family="ui-sans-serif,system-ui,sans-serif" font-size="18" font-weight="700">%s</text>
  <text x="16" y="48" fill="#a3988c" font-family="ui-monospace,monospace" font-size="12">%s</text>
</svg>`, html.EscapeString(label), tone, tone, html.EscapeString(label), html.EscapeString(sub)))
}
