import { demoResult, sleep, type Gate } from "./demo";
import { finishSample, type Sample } from "./metrics";
import { fraction, rpsAt, workersAt, type Preset } from "./preset";
import { assertGet, safeGet, type GetResult, type Policy } from "./safety";

export type Getter = (url: string) => Promise<GetResult>;

export function externalGetter(policy: Policy): Getter {
  return (url) => safeGet(url, policy, { maxBytes: 1 << 20, timeoutMs: 4000 });
}

export function demoGetter(gate: Gate): Getter {
  return async (raw) => {
    assertGet("GET");
    const planned = demoResult(new URL(raw), gate);
    const started = Date.now();
    await sleep(planned.delayMs);
    return {
      status: planned.status,
      body: planned.body,
      ms: Date.now() - started,
      contentType: planned.contentType,
      finalUrl: raw,
    };
  };
}

export async function probe(raw: string, getter: Getter): Promise<{
  pageMs: number;
  pageBytes: number;
  pageStatus: number;
  assets: { url: string; ms: number; bytes: number; contentType: string; status: number }[];
  error?: string;
}> {
  const out = { pageMs: 0, pageBytes: 0, pageStatus: 0, assets: [] as { url: string; ms: number; bytes: number; contentType: string; status: number }[] };
  let page: GetResult;
  try {
    page = await getter(raw);
  } catch (err) {
    return { ...out, error: err instanceof Error ? err.message : "probe failed" };
  }
  out.pageMs = page.ms;
  out.pageBytes = page.body.length;
  out.pageStatus = page.status;
  const base = new URL(page.finalUrl || raw);
  const seen = new Set<string>([base.pathname]);
  const attr = /(?:src|href)\s*=\s*["']([^"']+)["']/gi;
  const html = page.body.toString("utf8");
  const urls: string[] = [];
  for (const match of html.matchAll(attr)) {
    const ref = (match[1] || "").trim();
    if (!ref || ref.startsWith("data:") || ref.startsWith("mailto:") || ref.startsWith("#")) continue;
    let next: URL;
    try {
      next = new URL(ref, base);
    } catch {
      continue;
    }
    if (next.hostname !== base.hostname) continue;
    if (next.protocol !== "http:" && next.protocol !== "https:") continue;
    if (seen.has(next.pathname)) continue;
    seen.add(next.pathname);
    next.hash = "";
    urls.push(next.toString());
    if (urls.length === 4) break;
  }
  for (const assetUrl of urls) {
    try {
      const asset = await getter(assetUrl);
      out.assets.push({
        url: assetUrl,
        ms: asset.ms,
        bytes: asset.body.length,
        contentType: asset.contentType,
        status: asset.status,
      });
    } catch {
      out.assets.push({ url: assetUrl, ms: 0, bytes: 0, contentType: "", status: 0 });
    }
  }
  return out;
}

export async function runCurve(opts: {
  url: string;
  preset: Preset;
  maxRps: number;
  durationSeconds: number;
  policy: Policy;
  getter: Getter;
  killed: () => boolean | Promise<boolean>;
  signal?: AbortSignal;
  onSample?: (sample: Sample) => void | Promise<void>;
}): Promise<{ samples: Sample[]; cancelled: boolean }> {
  assertGet("GET");
  const seconds = Math.max(1, Math.min(opts.durationSeconds, opts.policy.maxDurationSeconds, 180));
  const cap = Math.min(opts.maxRps, opts.policy.maxRps);
  const ceiling = Math.ceil(opts.policy.maxRps * opts.policy.maxDurationSeconds) + 1;
  const samples: Sample[] = [];
  let sent = 0;
  let cancelled = false;
  for (let sec = 0; sec < seconds; sec++) {
    if (opts.signal?.aborted || (await opts.killed())) {
      cancelled = true;
      break;
    }
    const frac = fraction(sec, seconds);
    let workers = workersAt(opts.preset, frac);
    if (workers > opts.policy.maxWorkers) workers = opts.policy.maxWorkers;
    let rps = rpsAt(opts.preset, frac, cap);
    if (rps > opts.policy.maxRps) rps = opts.policy.maxRps;
    let n = Math.round(rps);
    if (sent + n > ceiling) n = Math.max(0, ceiling - sent);
    const sample = await fire(opts.url, n, workers, opts.getter, opts.signal);
    sample.sec = sec;
    samples.push(sample);
    sent += sample.requests;
    await opts.onSample?.(sample);
    if (sent >= ceiling) break;
  }
  return { samples, cancelled };
}

async function fire(url: string, n: number, workers: number, getter: Getter, signal?: AbortSignal): Promise<Sample> {
  if (n <= 0 || signal?.aborted) return finishSample(0, 0, [], workers, 0);
  const slots = Math.max(1, workers);
  let active = 0;
  const waiters: Array<() => void> = [];
  const latencies: number[] = [];
  let requests = 0;
  let errors = 0;
  const pace = 1000 / n;
  const started = Date.now();

  async function withSlot(fn: () => Promise<void>) {
    if (active >= slots) await new Promise<void>((resolve) => waiters.push(resolve));
    active += 1;
    try {
      await fn();
    } finally {
      active -= 1;
      waiters.shift()?.();
    }
  }

  const jobs: Promise<void>[] = [];
  for (let i = 0; i < n; i++) {
    if (signal?.aborted) break;
    const wait = started + i * pace - Date.now();
    if (wait > 0) await sleep(wait, signal);
    if (signal?.aborted) break;
    jobs.push(
      withSlot(async () => {
        assertGet("GET");
        const t0 = Date.now();
        try {
          const res = await getter(url);
          const ms = res.ms > 0 ? res.ms : Date.now() - t0;
          requests += 1;
          latencies.push(ms);
          if (res.status >= 400) errors += 1;
        } catch {
          requests += 1;
          errors += 1;
          latencies.push(Date.now() - t0);
        }
      }),
    );
  }
  await Promise.all(jobs);
  return finishSample(requests, errors, latencies, workers, 0);
}
