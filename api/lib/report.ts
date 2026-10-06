import type { Summary } from "./metrics";

// Order-of-magnitude Fluid Compute list prices published by Vercel:
// about $0.128 per active CPU-hour, $0.0106 per GB-hour, $0.60 per million invocations.
// Standard size here is 1 vCPU and 2 GB. Free tier is not subtracted.

const CPU_PER_HOUR = 0.128;
const MEM_PER_GB_HOUR = 0.0106;
const USD_PER_MILLION = 0.6;
const ASSUME_CONCURRENCY = 80;
const MEM_GB = 2;

const costNote =
  "Approximation of Vercel Functions Fluid Compute list price for a standard instance (about 1 vCPU and 2 GB): $0.128 per active CPU-hour, $0.0106 per GB-hour, and $0.60 per million invocations where the plan bills them. Two hours at the observed peak and ten hours at 20% of peak. Free tier and taxes are not subtracted. This is not an invoice.";

export type Fix = { title: string; detail: string };
export type Cost = { usd: number; instances: number; requests: number; note: string };
export type Asset = { url: string; ms: number; bytes: number; contentType: string; status: number };
export type Probe = { pageMs: number; pageBytes: number; pageStatus: number; assets: Asset[]; error?: string };

export type Report = {
  verdict: string;
  headline: string;
  summary: string;
  fixes: Fix[];
  degrades_past_rps: number;
  has_breaking: boolean;
  cost: Cost;
  model: string;
  notice?: string;
};

export type ReportInput = {
  url: string;
  presetId: string;
  presetName: string;
  maxRps: number;
  summary: Summary;
  probe: Probe;
};

export function template(input: ReportInput): Report {
  const s = input.summary;
  const rep: Report = {
    verdict: "",
    headline: "",
    summary: "",
    fixes: [],
    degrades_past_rps: s.breakingRps,
    has_breaking: s.hasBreaking,
    cost: estimate(s.peakRps, s.peakP95),
    model: "template",
  };
  const top5Line = 0.75 * input.maxRps;
  if (!s.hasBreaking && s.peakError < 0.02 && s.peakP95 < 800) {
    rep.verdict = "launch_ready";
    rep.headline = input.presetId === "numberone" ? "Survives a #1 Product of the Day launch" : "Survives a Top 5 launch";
    rep.summary = `Across this ${input.presetName || "preset"} shape, peak was ${s.peakRps.toFixed(0)} req/s with p95 ${s.peakP95.toFixed(0)} ms and ${(s.peakError * 100).toFixed(1)}% errors. Nothing crossed the breaking line.`;
  } else if (s.hasBreaking && s.breakingRps + 0.5 < top5Line) {
    rep.verdict = "fails";
    rep.headline = `Won't survive a Top 5 launch, ${degradeWord(s)} ~${s.breakingRps.toFixed(0)} req/s`;
    rep.summary = `The run broke at about ${s.breakingRps.toFixed(0)} req/s, which is under the Top 5 plateau. Peak p95 was ${s.peakP95.toFixed(0)} ms and the worst error rate was ${(s.peakError * 100).toFixed(1)}%.`;
  } else if (s.hasBreaking) {
    rep.verdict = "degraded";
    rep.headline = `Survives a Top 5 launch, ${degradeWord(s)} ~${s.breakingRps.toFixed(0)} req/s`;
    rep.summary = `The curve stayed healthy into the Top 5 band, then p95 or errors gave way around ${s.breakingRps.toFixed(0)} req/s. Worst p95 was ${s.peakP95.toFixed(0)} ms.`;
  } else {
    rep.verdict = "degraded";
    rep.headline = `Degraded through the run, worst p95 ${s.peakP95.toFixed(0)} ms`;
    rep.summary = "No single second crossed the breaking thresholds, but latency or errors were high enough that launch morning would feel rough.";
  }
  rep.fixes = fixes(input);
  if (rep.fixes.length === 0) {
    rep.fixes = [
      {
        title: "Hold one warm function for launch hour",
        detail:
          "Ping the route for the morning of the launch so the first visitors are not the ones who pay for a cold start.",
      },
    ];
  }
  return rep;
}

export function estimate(peakRps: number, peakP95Ms: number): Cost {
  const rps = Math.max(0, peakRps);
  const p95 = Math.max(0, peakP95Ms);
  const conc = rps * (p95 / 1000);
  let instances = Math.ceil(conc / ASSUME_CONCURRENCY);
  if (instances < 1 && rps > 0) instances = 1;
  let tail = Math.ceil(instances * 0.2);
  if (instances > 0 && tail < 1) tail = 1;
  const peakSec = 2 * 3600;
  const tailSec = 10 * 3600;
  const cpuSeconds = instances * peakSec + tail * tailSec;
  const cpuHours = cpuSeconds / 3600;
  const requests = Math.round(rps * peakSec + rps * 0.2 * tailSec);
  const usd = cpuHours * CPU_PER_HOUR + cpuHours * MEM_GB * MEM_PER_GB_HOUR + (requests / 1e6) * USD_PER_MILLION;
  return {
    usd: Math.round(usd * 100) / 100,
    instances,
    requests,
    note: costNote,
  };
}

function degradeWord(s: Summary): string {
  if (s.breakReason === "error" || (!s.breakReason && s.peakError > 0.05 && s.peakP95 <= 1500)) return "errors climb past";
  return "p95 degrades past";
}

function fixes(input: ReportInput): Fix[] {
  const out: Fix[] = [];
  const s = input.summary;
  let heavy: Asset | undefined;
  let slow: Asset | undefined;
  for (const asset of input.probe.assets) {
    if (isImage(asset) && (!heavy || asset.bytes > heavy.bytes)) heavy = asset;
    if (!slow || asset.ms > slow.ms) slow = asset;
  }
  if (heavy && heavy.bytes >= 100_000) {
    out.push({
      title: "Cut the image weight",
      detail: `${pathOf(heavy.url)} is ${Math.floor(heavy.bytes / 1024)} KB. Resize and compress it, and serve it from the CDN, before launch morning.`,
    });
  }
  if ((slow && slow.ms >= 700) || input.probe.pageMs >= 700) {
    let target = input.url;
    let ms = input.probe.pageMs;
    if (slow && slow.ms >= input.probe.pageMs && slow.url) {
      target = slow.url;
      ms = slow.ms;
    }
    out.push({
      title: "Speed up the slow endpoint",
      detail: `${pathOf(target)} took about ${ms.toFixed(0)} ms on a single quiet fetch. Cache it or move the work off the request path.`,
    });
  }
  if (s.peakP95 >= 400 || s.hasBreaking) {
    out.push({
      title: "Cache and put a CDN in front",
      detail:
        "p95 climbed under the ramp. Cache the HTML if it can be public, and serve static assets from the CDN so the function only sees the dynamic calls.",
    });
  }
  if (s.peakError > 0.01) {
    out.push({
      title: "Raise concurrency headroom",
      detail:
        "Errors showed up before the safety cap. Give the function more memory for launch morning and check that it can actually serve the concurrency you expect.",
    });
  }
  out.push({
    title: "Hold one warm function for launch hour",
    detail:
      "Scale-to-zero is right every other day. It is the wrong default at 12:01 AM Pacific. Keep one function warm until the front page cools off.",
  });
  return out;
}

function isImage(asset: Asset): boolean {
  if (asset.contentType.startsWith("image/")) return true;
  const p = asset.url.toLowerCase();
  return [".png", ".jpg", ".jpeg", ".webp", ".gif"].some((ext) => p.includes(ext));
}

function pathOf(raw: string): string {
  try {
    const path = new URL(raw).pathname;
    return path || "the page";
  } catch {
    return raw || "the page";
  }
}
