export function badgeSvg(status: string, verdict: string, p95: number, hasBreaking: boolean): string {
  let label = "In progress";
  let tone = "#8a8178";
  if (status === "failed") {
    label = "Run failed";
    tone = "#ff5d73";
  } else if (status === "cancelled") {
    label = "Cancelled";
    tone = "#ffb020";
  } else if (status === "completed") {
    if (verdict === "launch_ready" && !hasBreaking) {
      label = "Launch-ready";
      tone = "#c6f135";
    } else {
      label = "Needs work";
      tone = "#ff4d00";
    }
  }
  let sub = "Stampede";
  if (status === "completed") {
    if (hasBreaking) sub = "Broke under the ramp";
    else if (p95 > 0) sub = `p95 ${p95.toFixed(0)} ms`;
    else sub = "No breaking point";
  }
  const esc = (value: string) =>
    value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  return `<svg xmlns="http://www.w3.org/2000/svg" width="360" height="64" viewBox="0 0 360 64" role="img" aria-label="${esc(label)}">
  <rect width="360" height="64" rx="10" fill="#14110e"/>
  <rect x="1" y="1" width="358" height="62" rx="9" fill="none" stroke="${tone}" stroke-width="2"/>
  <text x="16" y="28" fill="${tone}" font-family="ui-sans-serif,system-ui,sans-serif" font-size="18" font-weight="700">${esc(label)}</text>
  <text x="16" y="48" fill="#a3988c" font-family="ui-monospace,monospace" font-size="12">${esc(sub)}</text>
</svg>`;
}
