// Product Hunt launch-day shapes. Magnitudes are scaled to the safety cap.
// The shapes are planning assumptions, not an official Product Hunt feed.

export const START_WORKERS = 10;
export const END_WORKERS = 50;

export type Preset = {
  id: string;
  name: string;
  blurb: string;
  assumptions: string[];
  durationSeconds: number;
  startWorkers: number;
  endWorkers: number;
  peak: number;
  intensity: (frac: number) => number;
};

const sharedAssumptions = [
  "These are planning shapes, not an official Product Hunt traffic feed.",
  "A typical maker page is about 10 HTTP requests per visitor (document plus assets).",
  "The Vercel demo compresses the launch morning into 30 seconds. The safety cap is still 3 minutes, and the local Go runner defaults to 45 seconds.",
  "Absolute rates are scaled to the safety cap (default 40 requests/second and 50 workers) so the shape is useful and the tool cannot be aimed as a flood.",
  "Top 5 of the Day assumes roughly 2,000–4,000 launch-day uniques. The plateau is 75% of the cap.",
  "Workers ramp from 10 to 50 across the run. On Vercel those are in-process workers inside one function.",
];

export function top5(durationSeconds: number): Preset {
  return {
    id: "top5",
    name: "Top 5 of the Day",
    blurb: "A front-page morning: steady climb, a plateau, then the crowd thins.",
    assumptions: [...sharedAssumptions],
    durationSeconds,
    startWorkers: START_WORKERS,
    endWorkers: END_WORKERS,
    peak: 0.75,
    intensity: top5Curve,
  };
}

export function numberOne(durationSeconds: number): Preset {
  return {
    id: "numberone",
    name: "#1 Product of the Day",
    blurb: "The sharp first-hour spike. Steeper than a Top 5, and it sits on the cap.",
    assumptions: [
      "Plan on roughly 6,000–15,000 launch-day uniques, with about a quarter of them in the first two hours.",
      ...sharedAssumptions,
    ],
    durationSeconds,
    startWorkers: START_WORKERS,
    endWorkers: END_WORKERS,
    peak: 1,
    intensity: numberOneCurve,
  };
}

export function allPresets(durationSeconds: number): Preset[] {
  return [top5(durationSeconds), numberOne(durationSeconds)];
}

export function presetById(id: string, durationSeconds: number): Preset | undefined {
  return allPresets(durationSeconds).find((p) => p.id === id);
}

export function fraction(sec: number, total: number): number {
  if (total <= 1) return 1;
  const clamped = Math.min(total - 1, Math.max(0, sec));
  return clamped / (total - 1);
}

export function workersAt(preset: Preset, frac: number): number {
  const f = clamp01(frac);
  const w = preset.startWorkers + f * (preset.endWorkers - preset.startWorkers);
  return Math.max(1, Math.round(w));
}

export function rpsAt(preset: Preset, frac: number, maxRps: number): number {
  return clamp01(preset.intensity(clamp01(frac))) * maxRps;
}

function top5Curve(f: number): number {
  if (f < 0.2) return lerp(0.22, 0.45, f / 0.2);
  if (f < 0.55) return lerp(0.45, 0.75, (f - 0.2) / 0.35);
  if (f < 0.8) return 0.75;
  return lerp(0.75, 0.48, (f - 0.8) / 0.2);
}

function numberOneCurve(f: number): number {
  if (f < 0.08) return lerp(0.3, 0.55, f / 0.08);
  if (f < 0.28) return lerp(0.55, 1, (f - 0.08) / 0.2);
  if (f < 0.68) return 1;
  return lerp(1, 0.62, (f - 0.68) / 0.32);
}

function lerp(a: number, b: number, t: number): number {
  const c = clamp01(t);
  return a + (b - a) * c;
}

function clamp01(f: number): number {
  if (f < 0) return 0;
  if (f > 1) return 1;
  return f;
}
