import type { Sample } from "./api";

type Props = {
  samples: Sample[];
  maxRps: number;
  breakingRps?: number;
  hasBreaking?: boolean;
};

export function Chart({ samples, maxRps, breakingRps, hasBreaking }: Props) {
  const w = 720;
  const h = 280;
  const pad = { l: 44, r: 16, t: 18, b: 28 };
  const innerW = w - pad.l - pad.r;
  const innerH = h - pad.t - pad.b;
  const n = Math.max(samples.length, 2);
  const rpsTop = Math.max(maxRps, ...samples.map((s) => s.rps), 1);
  const p95Top = Math.max(1500, ...samples.map((s) => s.p95_ms), 1);
  const x = (i: number) => pad.l + (i / (n - 1)) * innerW;
  const yRps = (v: number) => pad.t + innerH - (v / rpsTop) * innerH;
  const yP95 = (v: number) => pad.t + innerH - (v / p95Top) * innerH;
  const yWorkers = (v: number) => pad.t + innerH - (v / 50) * innerH;

  const line = (pts: string) => pts;
  const rpsPts = samples.map((s, i) => `${x(i)},${yRps(s.rps)}`).join(" ");
  const p95Pts = samples.map((s, i) => `${x(i)},${yP95(s.p95_ms)}`).join(" ");
  const workerArea =
    samples.length > 0
      ? `M ${x(0)} ${yWorkers(samples[0].workers)} ` +
        samples.map((s, i) => `L ${x(i)} ${yWorkers(s.workers)}`).join(" ") +
        ` L ${x(samples.length - 1)} ${pad.t + innerH} L ${x(0)} ${pad.t + innerH} Z`
      : "";
  const rpsArea =
    samples.length > 0
      ? `M ${x(0)} ${yRps(samples[0].rps)} ` +
        samples.map((s, i) => `L ${x(i)} ${yRps(s.rps)}`).join(" ") +
        ` L ${x(samples.length - 1)} ${pad.t + innerH} L ${x(0)} ${pad.t + innerH} Z`
      : "";

  let breakX: number | null = null;
  if (hasBreaking && breakingRps && samples.length) {
    const idx = samples.findIndex((s) => s.rps >= breakingRps - 0.5);
    breakX = x(idx >= 0 ? idx : samples.length - 1);
  }

  const latest = samples[samples.length - 1];
  const label = latest
    ? `Live chart. ${latest.rps.toFixed(0)} requests per second, p95 ${Math.round(latest.p95_ms)} milliseconds, ${latest.workers} workers, ${(latest.error_rate * 100).toFixed(1)} percent errors.`
    : "Live chart waiting for the first sample.";

  return (
    <svg className="chart" viewBox={`0 0 ${w} ${h}`} role="img" aria-label={label}>
      {[0, 0.25, 0.5, 0.75, 1].map((g) => (
        <g key={g}>
          <line
            x1={pad.l}
            x2={w - pad.r}
            y1={pad.t + innerH * (1 - g)}
            y2={pad.t + innerH * (1 - g)}
            className="grid"
          />
          <text x={pad.l - 8} y={pad.t + innerH * (1 - g) + 4} className="axis" textAnchor="end">
            {Math.round(rpsTop * g)}
          </text>
        </g>
      ))}
      {workerArea && <path d={workerArea} className="workers" />}
      {rpsArea && <path d={rpsArea} className="rps-fill" />}
      {rpsPts && <polyline points={line(rpsPts)} className="rps" />}
      {p95Pts && <polyline points={line(p95Pts)} className="p95" />}
      {breakX != null && (
        <line x1={breakX} x2={breakX} y1={pad.t} y2={pad.t + innerH} className="break" />
      )}
      <text x={pad.l} y={h - 8} className="axis">
        seconds
      </text>
    </svg>
  );
}
