export const ERROR_BREAK = 0.05;
export const P95_BREAK_MS = 1500;

export type Sample = {
  sec: number;
  workers: number;
  rps: number;
  p50_ms: number;
  p95_ms: number;
  error_rate: number;
  requests: number;
  errors: number;
};

export type Summary = {
  hasBreaking: boolean;
  breakingRps: number;
  peakRps: number;
  peakP95: number;
  peakError: number;
  requests: number;
  errors: number;
  breakReason?: "error" | "latency";
};

export function summarize(samples: Sample[]): Summary {
  const s: Summary = {
    hasBreaking: false,
    breakingRps: 0,
    peakRps: 0,
    peakP95: 0,
    peakError: 0,
    requests: 0,
    errors: 0,
  };
  for (const sm of samples) {
    if (sm.rps > s.peakRps) s.peakRps = sm.rps;
    if (sm.p95_ms > s.peakP95) s.peakP95 = sm.p95_ms;
    if (sm.error_rate > s.peakError) s.peakError = sm.error_rate;
    s.requests += sm.requests;
    s.errors += sm.errors;
    if (s.hasBreaking || sm.requests === 0) continue;
    if (sm.error_rate > ERROR_BREAK || sm.p95_ms > P95_BREAK_MS) {
      s.hasBreaking = true;
      s.breakingRps = sm.rps;
      s.breakReason = sm.error_rate > ERROR_BREAK ? "error" : "latency";
    }
  }
  return s;
}

export function percentile(sorted: number[], p: number): number {
  const n = sorted.length;
  if (n === 0) return 0;
  if (n === 1) return sorted[0] ?? 0;
  let idx = Math.ceil(p * n) - 1;
  if (idx < 0) idx = 0;
  if (idx >= n) idx = n - 1;
  return sorted[idx] ?? 0;
}

export function finishSample(
  requests: number,
  errors: number,
  latencies: number[],
  workers: number,
  sec: number,
): Sample {
  const sorted = [...latencies].sort((a, b) => a - b);
  const errorRate = requests > 0 ? errors / requests : 0;
  return {
    sec,
    workers,
    rps: requests > 0 ? requests : 0,
    p50_ms: percentile(sorted, 0.5),
    p95_ms: percentile(sorted, 0.95),
    error_rate: errorRate,
    requests,
    errors,
  };
}
