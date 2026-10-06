async function toRequest(req, fallbackProto = "http") {
  const host = header(req, "x-forwarded-host") || header(req, "host") || "localhost";
  const proto = header(req, "x-forwarded-proto") || fallbackProto;
  const url = new URL(req.url || "/", `${proto}://${host}`);
  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (typeof value === "string") headers.set(key, value);
    else if (Array.isArray(value)) value.forEach((item) => headers.append(key, item));
  }
  const method = req.method || "GET";
  if (method === "GET" || method === "HEAD") return new Request(url, { method, headers });
  const chunks = [];
  for await (const chunk of req) chunks.push(Buffer.from(chunk));
  return new Request(url, { method, headers, body: Buffer.concat(chunks) });
}
async function writeResponse(res, response) {
  res.statusCode = response.status;
  response.headers.forEach((value, key) => {
    res.setHeader(key, value);
  });
  if (response.headers.get("content-type")?.includes("text/event-stream")) {
    res.flushHeaders?.();
  }
  if (!response.body) {
    res.end();
    return;
  }
  const reader = response.body.getReader();
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      res.write(Buffer.from(value));
    }
  } finally {
    res.end();
  }
}
function header(req, name) {
  const value = req.headers[name];
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}

import { createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import { Resolver } from "node:dns/promises";

function badgeSvg(status, verdict, p95, hasBreaking) {
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
  const esc = (value) => value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  return `<svg xmlns="http://www.w3.org/2000/svg" width="360" height="64" viewBox="0 0 360 64" role="img" aria-label="${esc(label)}">
  <rect width="360" height="64" rx="10" fill="#14110e"/>
  <rect x="1" y="1" width="358" height="62" rx="9" fill="none" stroke="${tone}" stroke-width="2"/>
  <text x="16" y="28" fill="${tone}" font-family="ui-sans-serif,system-ui,sans-serif" font-size="18" font-weight="700">${esc(label)}</text>
  <text x="16" y="48" fill="#a3988c" font-family="ui-monospace,monospace" font-size="12">${esc(sub)}</text>
</svg>`;
}

var HERO_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  "base64"
);
var PAGE = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Northwind Kits</title></head>
<body>
  <h1>Northwind Kits</h1>
  <p>A small shop, about to meet the front page.</p>
  <img src="/demo/hero.png" alt="Kit on a bench" width="480" height="270">
  <p><a href="/demo/api/pricing">Pricing</a></p>
</body></html>`;
function createGate() {
  const hits = [];
  return {
    hit() {
      const now = Date.now();
      const cut = now - 1e3;
      while (hits.length && hits[0] <= cut) hits.shift();
      hits.push(now);
      const n = hits.length;
      if (n >= 34) return { delayMs: 30, fail: true };
      if (n >= 24) return { delayMs: 1600, fail: false };
      if (n >= 14) return { delayMs: 220, fail: false };
      return { delayMs: 20, fail: false };
    }
  };
}
var demoGate = createGate();
function demoResult(url, gate = demoGate) {
  const path = url.pathname;
  if (path.startsWith("/.well-known/stampede-") && path.endsWith(".txt")) {
    const token = path.slice("/.well-known/stampede-".length, -".txt".length);
    if (token.length < 6 || token.length > 80 || /[/\\]/.test(token)) {
      return { status: 404, body: Buffer.from("not found"), contentType: "text/plain; charset=utf-8", delayMs: 0 };
    }
    return { status: 200, body: Buffer.from(token), contentType: "text/plain; charset=utf-8", delayMs: 0 };
  }
  if (path === "/demo/hero.png") {
    return { status: 200, body: HERO_PNG, contentType: "image/png", delayMs: 0 };
  }
  if (path === "/demo/api/pricing") {
    return {
      status: 200,
      body: Buffer.from(`{"plan":"launch","amount":49}`),
      contentType: "application/json",
      delayMs: 900
    };
  }
  if (path === "/demo" || path === "/demo/") {
    const decision = gate.hit();
    if (decision.fail) {
      return {
        status: 503,
        body: Buffer.from("overloaded"),
        contentType: "text/plain; charset=utf-8",
        delayMs: decision.delayMs
      };
    }
    return {
      status: 200,
      body: Buffer.from(PAGE),
      contentType: "text/html; charset=utf-8",
      delayMs: decision.delayMs
    };
  }
  return { status: 404, body: Buffer.from("not found"), contentType: "text/plain; charset=utf-8", delayMs: 0 };
}
function isDemoPath(url, selfHost) {
  return url.hostname.toLowerCase() === selfHost.toLowerCase() && (url.pathname === "/demo" || url.pathname === "/demo/");
}
async function sleep(ms, signal) {
  if (ms <= 0) return;
  await new Promise((resolve) => {
    const timer = setTimeout(resolve, ms);
    const onAbort = () => {
      clearTimeout(timer);
      resolve();
    };
    if (signal?.aborted) {
      onAbort();
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

var MAX_RPS = 40;
var MAX_WORKERS = 50;
var SAFETY_MAX_SECONDS = 180;
var DEFAULT_PLATFORM_MAX_SECONDS = 40;
var DEFAULT_DURATION_SECONDS = 30;
var DOMAIN_QUOTA = 3;
var IP_QUOTA = 5;
var CHALLENGE_LIMIT_PER_HOUR = 30;
var USER_AGENT = "StampedeBot/1.0 (launch check; ownership-verified GET only)";
function intEnv(name, fallback, min, max) {
  const raw = process.env[name];
  if (!raw) return fallback;
  const n = Number(raw);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, Math.floor(n)));
}
function platformMaxSeconds() {
  return intEnv("STAMPEDE_PLATFORM_MAX_SECONDS", DEFAULT_PLATFORM_MAX_SECONDS, 10, SAFETY_MAX_SECONDS);
}
function defaultDurationSeconds() {
  return intEnv("STAMPEDE_DURATION_SECONDS", DEFAULT_DURATION_SECONDS, 10, platformMaxSeconds());
}
function maxRps() {
  const raw = process.env.STAMPEDE_MAX_RPS;
  if (!raw) return MAX_RPS;
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0) return MAX_RPS;
  return Math.min(MAX_RPS, n);
}
function allowHosts() {
  const out = /* @__PURE__ */ new Set();
  for (const part of (process.env.STAMPEDE_ALLOW_HOSTS || "").split(",")) {
    const host = part.trim().toLowerCase().replace(/\.$/, "");
    if (host) out.add(host);
  }
  return out;
}

var ERROR_BREAK = 0.05;
var P95_BREAK_MS = 1500;
function summarize(samples) {
  const s = {
    hasBreaking: false,
    breakingRps: 0,
    peakRps: 0,
    peakP95: 0,
    peakError: 0,
    requests: 0,
    errors: 0
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
function percentile(sorted, p) {
  const n = sorted.length;
  if (n === 0) return 0;
  if (n === 1) return sorted[0] ?? 0;
  let idx = Math.ceil(p * n) - 1;
  if (idx < 0) idx = 0;
  if (idx >= n) idx = n - 1;
  return sorted[idx] ?? 0;
}
function finishSample(requests, errors, latencies, workers, sec) {
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
    errors
  };
}

var START_WORKERS = 10;
var END_WORKERS = 50;
var sharedAssumptions = [
  "These are planning shapes, not an official Product Hunt traffic feed.",
  "A typical maker page is about 10 HTTP requests per visitor (document plus assets).",
  "The Vercel demo compresses the launch morning into 30 seconds. The safety cap is still 3 minutes, and the local Go runner defaults to 45 seconds.",
  "Absolute rates are scaled to the safety cap (default 40 requests/second and 50 workers) so the shape is useful and the tool cannot be aimed as a flood.",
  "Top 5 of the Day assumes roughly 2,000\u20134,000 launch-day uniques. The plateau is 75% of the cap.",
  "Workers ramp from 10 to 50 across the run. On Vercel those are in-process workers inside one function."
];
function top5(durationSeconds) {
  return {
    id: "top5",
    name: "Top 5 of the Day",
    blurb: "A front-page morning: steady climb, a plateau, then the crowd thins.",
    assumptions: [...sharedAssumptions],
    durationSeconds,
    startWorkers: START_WORKERS,
    endWorkers: END_WORKERS,
    peak: 0.75,
    intensity: top5Curve
  };
}
function numberOne(durationSeconds) {
  return {
    id: "numberone",
    name: "#1 Product of the Day",
    blurb: "The sharp first-hour spike. Steeper than a Top 5, and it sits on the cap.",
    assumptions: [
      "Plan on roughly 6,000\u201315,000 launch-day uniques, with about a quarter of them in the first two hours.",
      ...sharedAssumptions
    ],
    durationSeconds,
    startWorkers: START_WORKERS,
    endWorkers: END_WORKERS,
    peak: 1,
    intensity: numberOneCurve
  };
}
function allPresets(durationSeconds) {
  return [top5(durationSeconds), numberOne(durationSeconds)];
}
function presetById(id, durationSeconds) {
  return allPresets(durationSeconds).find((p) => p.id === id);
}
function fraction(sec, total) {
  if (total <= 1) return 1;
  const clamped = Math.min(total - 1, Math.max(0, sec));
  return clamped / (total - 1);
}
function workersAt(preset, frac) {
  const f = clamp01(frac);
  const w = preset.startWorkers + f * (preset.endWorkers - preset.startWorkers);
  return Math.max(1, Math.round(w));
}
function rpsAt(preset, frac, maxRps2) {
  return clamp01(preset.intensity(clamp01(frac))) * maxRps2;
}
function top5Curve(f) {
  if (f < 0.2) return lerp(0.22, 0.45, f / 0.2);
  if (f < 0.55) return lerp(0.45, 0.75, (f - 0.2) / 0.35);
  if (f < 0.8) return 0.75;
  return lerp(0.75, 0.48, (f - 0.8) / 0.2);
}
function numberOneCurve(f) {
  if (f < 0.08) return lerp(0.3, 0.55, f / 0.08);
  if (f < 0.28) return lerp(0.55, 1, (f - 0.08) / 0.2);
  if (f < 0.68) return 1;
  return lerp(1, 0.62, (f - 0.68) / 0.32);
}
function lerp(a, b, t) {
  const c = clamp01(t);
  return a + (b - a) * c;
}
function clamp01(f) {
  if (f < 0) return 0;
  if (f > 1) return 1;
  return f;
}

import { lookup } from "node:dns/promises";
import http from "node:http";
import https from "node:https";
import { BlockList, isIP } from "node:net";
var PolicyError = class extends Error {
  constructor(message) {
    super(message);
    this.name = "PolicyError";
  }
};
function defaultPolicy(over = {}) {
  return {
    allowHosts: over.allowHosts ?? /* @__PURE__ */ new Set(),
    maxRps: over.maxRps ?? MAX_RPS,
    maxDurationSeconds: over.maxDurationSeconds ?? SAFETY_MAX_SECONDS,
    maxWorkers: over.maxWorkers ?? MAX_WORKERS,
    maxRedirects: over.maxRedirects ?? 2,
    resolver: over.resolver
  };
}
var blocked = new BlockList();
for (const [addr, prefix, type] of [
  ["0.0.0.0", 8, "ipv4"],
  ["10.0.0.0", 8, "ipv4"],
  ["100.64.0.0", 10, "ipv4"],
  ["127.0.0.0", 8, "ipv4"],
  ["169.254.0.0", 16, "ipv4"],
  ["172.16.0.0", 12, "ipv4"],
  ["192.0.0.0", 24, "ipv4"],
  ["192.0.2.0", 24, "ipv4"],
  ["192.168.0.0", 16, "ipv4"],
  ["198.18.0.0", 15, "ipv4"],
  ["198.51.100.0", 24, "ipv4"],
  ["203.0.113.0", 24, "ipv4"],
  ["224.0.0.0", 4, "ipv4"],
  ["240.0.0.0", 4, "ipv4"],
  ["::1", 128, "ipv6"],
  ["fc00::", 7, "ipv6"],
  ["fe80::", 10, "ipv6"],
  ["2001:db8::", 32, "ipv6"]
]) {
  blocked.addSubnet(addr, prefix, type);
}
function normalize(raw, policy) {
  const trimmed = raw.trim();
  if (!trimmed || trimmed.length > 2048 || /[\\\n\r]/.test(trimmed)) {
    throw new PolicyError("host is not allowed");
  }
  let url;
  try {
    url = new URL(trimmed);
  } catch {
    throw new PolicyError("host is not allowed");
  }
  if (url.username || url.password) throw new PolicyError("URLs with userinfo are blocked");
  const scheme = url.protocol.replace(":", "").toLowerCase();
  if (scheme !== "http" && scheme !== "https") throw new PolicyError("only http and https URLs are allowed");
  const host = normalizeHost(url.hostname);
  if (!host) throw new PolicyError("host is not allowed");
  if (isMetadataHost(host)) throw new PolicyError("cloud metadata address blocked");
  if (isObfuscatedHost(host)) throw new PolicyError("obfuscated IP host is blocked");
  let port = scheme === "https" ? 443 : 80;
  if (url.port) {
    const n = Number(url.port);
    if (!Number.isInteger(n) || n < 1 || n > 65535) throw new PolicyError("port is not allowed");
    port = n;
  }
  const allow = policy.allowHosts.has(host);
  if (isIP(host)) assertIp(host, allow);
  else if (!host.includes(".") && !allow) throw new PolicyError("host is not allowed");
  if (!allowedPort(port, allow)) throw new PolicyError("port is not allowed");
  url.hash = "";
  url.protocol = `${scheme}:`;
  url.hostname = host;
  url.port = port === 80 && scheme === "http" || port === 443 && scheme === "https" ? "" : String(port);
  return { url, host, port };
}
function validatePlan(policy, rps, seconds, workers) {
  if (rps <= 0 || seconds < 1 || workers <= 0) throw new PolicyError("requested load exceeds the safety cap");
  if (rps > policy.maxRps || seconds > policy.maxDurationSeconds || workers > policy.maxWorkers) {
    throw new PolicyError("requested load exceeds the safety cap");
  }
  if (seconds > SAFETY_MAX_SECONDS) throw new PolicyError("requested load exceeds the safety cap");
}
function assertGet(method) {
  if (method !== "GET") throw new PolicyError("only GET is allowed");
}
async function safeGet(raw, policy, opts = {}) {
  const maxBytes = opts.maxBytes ?? 1 << 20;
  const timeoutMs = opts.timeoutMs ?? 8e3;
  let current = normalize(raw, policy);
  for (let hop = 0; hop <= policy.maxRedirects; hop++) {
    const ip = await resolveHost(current.host, policy);
    const result = await requestOnce(current, ip, maxBytes, timeoutMs);
    const location = result.location;
    if (result.status >= 300 && result.status < 400 && location) {
      if (hop === policy.maxRedirects) throw new PolicyError("redirect blocked");
      const next = new URL(location, current.url);
      if (normalizeHost(next.hostname) !== current.host) throw new PolicyError("redirect blocked");
      current = normalize(next.toString(), policy);
      continue;
    }
    return {
      status: result.status,
      body: result.body,
      ms: result.ms,
      contentType: result.contentType,
      finalUrl: current.url.toString()
    };
  }
  throw new PolicyError("redirect blocked");
}
async function resolveHost(host, policy) {
  const allow = policy.allowHosts.has(normalizeHost(host));
  if (isMetadataHost(host)) throw new PolicyError("cloud metadata address blocked");
  if (isIP(host)) {
    assertIp(host, allow);
    return canonical(host).addr;
  }
  const addrs = policy.resolver ? await policy.resolver(host) : (await lookup(host, { all: true, verbatim: true })).map((a) => a.address);
  if (!addrs.length) throw new PolicyError("host is not allowed");
  let picked = "";
  for (const addr of addrs) {
    assertIp(addr, allow);
    const kind = isIP(addr);
    if (kind === 4) picked = canonical(addr).addr;
    else if (!picked) picked = canonical(addr).addr;
  }
  if (!picked) throw new PolicyError("private or internal address blocked");
  return picked;
}
function requestOnce(target, ip, maxBytes, timeoutMs) {
  assertGet("GET");
  const started = Date.now();
  const lib = target.url.protocol === "https:" ? https : http;
  const path = `${target.url.pathname}${target.url.search}` || "/";
  return new Promise((resolve, reject) => {
    const req = lib.request(
      {
        host: ip,
        port: target.port,
        path,
        method: "GET",
        headers: {
          Host: target.url.host,
          "User-Agent": USER_AGENT,
          Accept: "text/html,application/xhtml+xml,text/plain,*/*;q=0.8",
          Connection: "close"
        },
        servername: target.host,
        timeout: timeoutMs,
        agent: false
      },
      (res) => {
        const chunks = [];
        let size = 0;
        res.on("data", (chunk) => {
          size += chunk.length;
          if (size <= maxBytes) chunks.push(chunk);
          else res.destroy();
        });
        res.on("end", () => {
          resolve({
            status: res.statusCode || 0,
            body: Buffer.concat(chunks),
            ms: Date.now() - started,
            contentType: String(res.headers["content-type"] || ""),
            location: headerOne(res.headers.location)
          });
        });
        res.on("error", reject);
      }
    );
    req.on("timeout", () => req.destroy(new Error("timeout")));
    req.on("error", reject);
    req.end();
  });
}
function headerOne(value) {
  if (Array.isArray(value)) return value[0];
  return value;
}
function assertIp(addr, allow) {
  const { addr: ip, type } = canonical(addr);
  if (isMetadataIp(ip)) throw new PolicyError("cloud metadata address blocked");
  if (!allow && blocked.check(ip, type)) throw new PolicyError("private or internal address blocked");
}
function canonical(addr) {
  const lower = addr.toLowerCase();
  if (lower.startsWith("::ffff:")) {
    const v4 = lower.slice(7);
    if (isIP(v4) === 4) return { addr: v4, type: "ipv4" };
  }
  return { addr: lower, type: isIP(lower) === 6 ? "ipv6" : "ipv4" };
}
function isMetadataIp(ip) {
  const parts = ip.split(".").map((n) => Number(n));
  if (parts.length !== 4 || parts.some((n) => !Number.isInteger(n))) return false;
  return parts[0] === 169 && parts[1] === 254 && parts[2] === 169 && (parts[3] === 254 || parts[3] === 253);
}
function isMetadataHost(host) {
  switch (normalizeHost(host)) {
    case "metadata.google.internal":
    case "metadata.goog":
    case "metadata":
      return true;
    default:
      return false;
  }
}
function isObfuscatedHost(host) {
  return host.length > 0 && /^[0-9]+$/.test(host);
}
function allowedPort(port, allowlisted) {
  if (port < 1 || port > 65535) return false;
  if (allowlisted) return true;
  return port === 80 || port === 443;
}
function normalizeHost(host) {
  return host.toLowerCase().trim().replace(/\.$/, "");
}

function externalGetter(policy) {
  return (url) => safeGet(url, policy, { maxBytes: 1 << 20, timeoutMs: 4e3 });
}
function demoGetter(gate) {
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
      finalUrl: raw
    };
  };
}
async function probe(raw, getter) {
  const out = { pageMs: 0, pageBytes: 0, pageStatus: 0, assets: [] };
  let page;
  try {
    page = await getter(raw);
  } catch (err) {
    return { ...out, error: err instanceof Error ? err.message : "probe failed" };
  }
  out.pageMs = page.ms;
  out.pageBytes = page.body.length;
  out.pageStatus = page.status;
  const base = new URL(page.finalUrl || raw);
  const seen = /* @__PURE__ */ new Set([base.pathname]);
  const attr = /(?:src|href)\s*=\s*["']([^"']+)["']/gi;
  const html = page.body.toString("utf8");
  const urls = [];
  for (const match of html.matchAll(attr)) {
    const ref = (match[1] || "").trim();
    if (!ref || ref.startsWith("data:") || ref.startsWith("mailto:") || ref.startsWith("#")) continue;
    let next;
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
        status: asset.status
      });
    } catch {
      out.assets.push({ url: assetUrl, ms: 0, bytes: 0, contentType: "", status: 0 });
    }
  }
  return out;
}
async function runCurve(opts) {
  assertGet("GET");
  const seconds = Math.max(1, Math.min(opts.durationSeconds, opts.policy.maxDurationSeconds, 180));
  const cap = Math.min(opts.maxRps, opts.policy.maxRps);
  const ceiling = Math.ceil(opts.policy.maxRps * opts.policy.maxDurationSeconds) + 1;
  const samples = [];
  let sent = 0;
  let cancelled = false;
  for (let sec = 0; sec < seconds; sec++) {
    if (opts.signal?.aborted || await opts.killed()) {
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
async function fire(url, n, workers, getter, signal) {
  if (n <= 0 || signal?.aborted) return finishSample(0, 0, [], workers, 0);
  const slots = Math.max(1, workers);
  let active = 0;
  const waiters = [];
  const latencies = [];
  let requests = 0;
  let errors = 0;
  const pace = 1e3 / n;
  const started = Date.now();
  async function withSlot(fn) {
    if (active >= slots) await new Promise((resolve) => waiters.push(resolve));
    active += 1;
    try {
      await fn();
    } finally {
      active -= 1;
      waiters.shift()?.();
    }
  }
  const jobs = [];
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
      })
    );
  }
  await Promise.all(jobs);
  return finishSample(requests, errors, latencies, workers, 0);
}

var CPU_PER_HOUR = 0.128;
var MEM_PER_GB_HOUR = 0.0106;
var USD_PER_MILLION = 0.6;
var ASSUME_CONCURRENCY = 80;
var MEM_GB = 2;
var costNote = "Approximation of Vercel Functions Fluid Compute list price for a standard instance (about 1 vCPU and 2 GB): $0.128 per active CPU-hour, $0.0106 per GB-hour, and $0.60 per million invocations where the plan bills them. Two hours at the observed peak and ten hours at 20% of peak. Free tier and taxes are not subtracted. This is not an invoice.";
function template(input) {
  const s = input.summary;
  const rep = {
    verdict: "",
    headline: "",
    summary: "",
    fixes: [],
    degrades_past_rps: s.breakingRps,
    has_breaking: s.hasBreaking,
    cost: estimate(s.peakRps, s.peakP95),
    model: "template"
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
        detail: "Ping the route for the morning of the launch so the first visitors are not the ones who pay for a cold start."
      }
    ];
  }
  return rep;
}
function estimate(peakRps, peakP95Ms) {
  const rps = Math.max(0, peakRps);
  const p95 = Math.max(0, peakP95Ms);
  const conc = rps * (p95 / 1e3);
  let instances = Math.ceil(conc / ASSUME_CONCURRENCY);
  if (instances < 1 && rps > 0) instances = 1;
  let tail = Math.ceil(instances * 0.2);
  if (instances > 0 && tail < 1) tail = 1;
  const peakSec = 2 * 3600;
  const tailSec = 10 * 3600;
  const cpuSeconds = instances * peakSec + tail * tailSec;
  const cpuHours = cpuSeconds / 3600;
  const requests = Math.round(rps * peakSec + rps * 0.2 * tailSec);
  const usd = cpuHours * CPU_PER_HOUR + cpuHours * MEM_GB * MEM_PER_GB_HOUR + requests / 1e6 * USD_PER_MILLION;
  return {
    usd: Math.round(usd * 100) / 100,
    instances,
    requests,
    note: costNote
  };
}
function degradeWord(s) {
  if (s.breakReason === "error" || !s.breakReason && s.peakError > 0.05 && s.peakP95 <= 1500) return "errors climb past";
  return "p95 degrades past";
}
function fixes(input) {
  const out = [];
  const s = input.summary;
  let heavy;
  let slow;
  for (const asset of input.probe.assets) {
    if (isImage(asset) && (!heavy || asset.bytes > heavy.bytes)) heavy = asset;
    if (!slow || asset.ms > slow.ms) slow = asset;
  }
  if (heavy && heavy.bytes >= 1e5) {
    out.push({
      title: "Cut the image weight",
      detail: `${pathOf(heavy.url)} is ${Math.floor(heavy.bytes / 1024)} KB. Resize and compress it, and serve it from the CDN, before launch morning.`
    });
  }
  if (slow && slow.ms >= 700 || input.probe.pageMs >= 700) {
    let target = input.url;
    let ms = input.probe.pageMs;
    if (slow && slow.ms >= input.probe.pageMs && slow.url) {
      target = slow.url;
      ms = slow.ms;
    }
    out.push({
      title: "Speed up the slow endpoint",
      detail: `${pathOf(target)} took about ${ms.toFixed(0)} ms on a single quiet fetch. Cache it or move the work off the request path.`
    });
  }
  if (s.peakP95 >= 400 || s.hasBreaking) {
    out.push({
      title: "Cache and put a CDN in front",
      detail: "p95 climbed under the ramp. Cache the HTML if it can be public, and serve static assets from the CDN so the function only sees the dynamic calls."
    });
  }
  if (s.peakError > 0.01) {
    out.push({
      title: "Raise concurrency headroom",
      detail: "Errors showed up before the safety cap. Give the function more memory for launch morning and check that it can actually serve the concurrency you expect."
    });
  }
  out.push({
    title: "Hold one warm function for launch hour",
    detail: "Scale-to-zero is right every other day. It is the wrong default at 12:01 AM Pacific. Keep one function warm until the front page cools off."
  });
  return out;
}
function isImage(asset) {
  if (asset.contentType.startsWith("image/")) return true;
  const p = asset.url.toLowerCase();
  return [".png", ".jpg", ".jpeg", ".webp", ".gif"].some((ext) => p.includes(ext));
}
function pathOf(raw) {
  try {
    const path = new URL(raw).pathname;
    return path || "the page";
  } catch {
    return raw || "the page";
  }
}

function memoryStore() {
  const map = /* @__PURE__ */ new Map();
  return {
    async get(key) {
      const entry = map.get(key);
      if (!entry) return null;
      if (entry.exp <= Date.now()) {
        map.delete(key);
        return null;
      }
      return entry.value;
    },
    async set(key, value, ttlSeconds) {
      map.set(key, { value, exp: Date.now() + ttlSeconds * 1e3 });
    }
  };
}
function runtimeStore() {
  const mem = memoryStore();
  if (process.env.VERCEL !== "1") return mem;
  return {
    async get(key) {
      const local = await mem.get(key);
      if (local != null) return local;
      const remote = await remoteGet(key);
      if (remote != null) {
        await mem.set(key, remote, 60);
        return remote;
      }
      return null;
    },
    async set(key, value, ttlSeconds) {
      await mem.set(key, value, ttlSeconds);
      await remoteSet(key, value, ttlSeconds);
    }
  };
}
async function remoteGet(key) {
  try {
    const { getCache } = await import("@vercel/functions");
    const value = await getCache({ namespace: "stampede" }).get(key);
    return typeof value === "string" ? value : null;
  } catch (err) {
    console.error("runtime cache get failed", err);
    return null;
  }
}
async function remoteSet(key, value, ttlSeconds) {
  try {
    const { getCache } = await import("@vercel/functions");
    await getCache({ namespace: "stampede" }).set(key, value, { ttl: ttlSeconds, tags: ["stampede"] });
  } catch (err) {
    console.error("runtime cache set failed", err);
  }
}

function createApp(options = {}) {
  const store = options.store ?? runtimeStore();
  const policy = options.policy ?? defaultPolicy({ allowHosts: allowHosts(), maxRps: maxRps() });
  const platformMax = options.platformMaxSeconds ?? platformMaxSeconds();
  const presetDuration = options.defaultDuration ?? defaultDurationSeconds();
  const domainQuota = options.domainQuota ?? DOMAIN_QUOTA;
  const ipQuota = options.ipQuota ?? IP_QUOTA;
  const adminToken = options.adminToken ?? process.env.STAMPEDE_ADMIN_TOKEN ?? "";
  const now = options.now ?? (() => Date.now());
  const trustProxy = options.trustProxy ?? (process.env.VERCEL === "1" || process.env.TRUST_PROXY === "1");
  async function handle(request) {
    try {
      const url = new URL(request.url);
      const path = url.pathname.replace(/^\/api(?=\/)/, "") || "/";
      if (request.method === "GET" && (path === "/healthz" || path === "/v1/healthz")) {
        return json({ status: "ok" });
      }
      if (request.method === "GET" && path === "/v1/config") return await config2(request);
      if (request.method === "GET" && path === "/v1/presets") return presets();
      if (request.method === "POST" && path === "/v1/challenges") return await createChallenge(request);
      const challengeGet = path.match(/^\/v1\/challenges\/([^/]+)$/);
      if (request.method === "GET" && challengeGet?.[1]) return await getChallenge(decodeURIComponent(challengeGet[1]));
      const challengeVerify = path.match(/^\/v1\/challenges\/([^/]+)\/verify$/);
      if (request.method === "POST" && challengeVerify?.[1]) {
        return await verifyChallenge(request, decodeURIComponent(challengeVerify[1]));
      }
      if (request.method === "POST" && path === "/v1/runs") return await createRun(request);
      const events = path.match(/^\/v1\/runs\/([^/]+)\/events$/);
      if (request.method === "GET" && events?.[1]) return runEvents(decodeURIComponent(events[1]));
      const badge = path.match(/^\/v1\/runs\/([^/]+)\/badge\.svg$/);
      if (request.method === "GET" && badge?.[1]) return await runBadge(decodeURIComponent(badge[1]));
      const run = path.match(/^\/v1\/runs\/([^/]+)$/);
      if (request.method === "GET" && run?.[1]) return await getRun(decodeURIComponent(run[1]));
      if (request.method === "POST" && path === "/v1/admin/kill") return await setKill(request, true);
      if (request.method === "POST" && path === "/v1/admin/resume") return await setKill(request, false);
      return json({ error: "not found" }, 404);
    } catch (err) {
      if (err instanceof HttpError) return json({ error: err.message }, err.status);
      if (err instanceof PolicyError) return json({ error: err.message }, 400);
      console.error(err);
      return json({ error: "internal error" }, 500);
    }
  }
  function selfHost(request) {
    return normalizeHost(new URL(request.url).hostname);
  }
  function normalizeForRequest(raw, request) {
    try {
      return normalize(raw, policy);
    } catch (err) {
      let host = "";
      try {
        host = normalizeHost(new URL(raw).hostname);
      } catch {
        throw err;
      }
      if (!host || host !== selfHost(request)) throw err;
      const allow = new Set(policy.allowHosts);
      allow.add(host);
      return normalize(raw, { ...policy, allowHosts: allow });
    }
  }
  function demoUrl(request) {
    const url = new URL(request.url);
    return `${url.protocol}//${url.host}/demo`;
  }
  async function killed() {
    if (process.env.KILL_SWITCH === "1") return true;
    return await store.get("kill") === "1";
  }
  async function config2(request) {
    return json({
      demo_url: demoUrl(request),
      max_rps: policy.maxRps,
      max_duration_seconds: SAFETY_MAX_SECONDS,
      platform_max_seconds: platformMax,
      preset_seconds: presetDuration,
      max_workers: policy.maxWorkers,
      kill: await killed(),
      model: "template",
      platform: "vercel",
      scheduler: false,
      domain_quota: domainQuota,
      ip_quota: ipQuota
    });
  }
  function presets() {
    return json({
      presets: allPresets(presetDuration).map((p) => ({
        id: p.id,
        name: p.name,
        blurb: p.blurb,
        assumptions: p.assumptions,
        duration_seconds: p.durationSeconds,
        start_workers: p.startWorkers,
        end_workers: p.endWorkers,
        peak_fraction: p.peak
      }))
    });
  }
  async function createChallenge(request) {
    const body = await readJson(request);
    const raw = typeof body.url === "string" ? body.url : "";
    const target = normalizeForRequest(raw, request);
    assertOwnHostPath(target.url, selfHost(request));
    const ip = clientIp(request, trustProxy);
    const hour = new Date(now()).toISOString().slice(0, 13);
    const rateKey = `chrate:${ip}:${hour}`;
    const used = Number(await store.get(rateKey) || "0");
    if (used >= CHALLENGE_LIMIT_PER_HOUR) {
      throw new HttpError(429, "too many verification challenges from this network");
    }
    await store.set(rateKey, String(used + 1), 3700);
    const claim = {
      u: target.url.toString(),
      h: target.host,
      t: `st_${randomBytes(16).toString("hex")}`,
      e: now() + 30 * 60 * 1e3,
      ip
    };
    const id = sign(claim);
    await store.set(`ch:${id}`, JSON.stringify({ verified: false, method: "" }), 30 * 60);
    return json(challengeView(id, claim, false, ""), 201);
  }
  async function getChallenge(id) {
    const claim = readClaim(id, now());
    const saved = await readFlag(id);
    return json(challengeView(id, claim, saved.verified, saved.method));
  }
  async function verifyChallenge(request, id) {
    const body = await readJson(request);
    const method = body.method === "dns" ? "dns" : body.method === "file" ? "file" : "";
    if (!method) throw new HttpError(400, "method must be file or dns");
    const claim = readClaim(id, now());
    const saved = await readFlag(id);
    if (saved.verified) return json({ verified: true, method: saved.method, grant: saved.grant });
    if (method === "file") await checkFile(request, claim);
    else await checkDns(claim);
    const grant = signGrant({
      id,
      u: claim.u,
      h: claim.h,
      t: claim.t,
      m: method,
      e: now() + 24 * 60 * 60 * 1e3
    });
    await store.set(`ch:${id}`, JSON.stringify({ verified: true, method, grant }), 24 * 60 * 60);
    return json({ verified: true, method, grant });
  }
  async function createRun(request) {
    if (await killed()) throw new HttpError(409, "stampede is paused");
    const body = await readJson(request);
    const id = typeof body.challenge_id === "string" ? body.challenge_id : "";
    const presetId = typeof body.preset === "string" ? body.preset : "";
    const claim = readClaim(id, now());
    const preset = presetById(presetId, presetDuration);
    if (!preset) throw new HttpError(400, "unknown preset");
    const saved = await readFlag(id);
    const grantToken = typeof body.grant === "string" && body.grant ? body.grant : saved.grant;
    const grant = grantToken ? readGrant(grantToken, now()) : null;
    const allowed = saved.verified || grant != null && grant.id === id && grant.u === claim.u;
    if (!allowed) throw new HttpError(403, "ownership verification required");
    const method = saved.method || grant?.m || "file";
    const page = new URL(claim.u);
    if (normalizeHost(page.hostname) === selfHost(request) && !isDemoPath(page, selfHost(request))) {
      throw new HttpError(400, "on this host, the only site Stampede will test is the demo shop");
    }
    if (!isDemoPath(page, selfHost(request))) {
      if (method === "dns") await checkDns(claim);
      else await checkFile(request, claim);
    }
    let seconds = preset.durationSeconds;
    if (typeof body.duration_seconds === "number" && body.duration_seconds > 0) seconds = Math.floor(body.duration_seconds);
    if (seconds > platformMax) {
      throw new HttpError(400, `this Vercel deployment runs curves up to ${platformMax} seconds so they finish within the function limit`);
    }
    validatePlan(policy, policy.maxRps, seconds, Math.min(MAX_WORKERS, policy.maxWorkers));
    const ip = clientIp(request, trustProxy);
    const day = new Date(now()).toISOString().slice(0, 10);
    const hostKey = `q:host:${claim.h}:${day}`;
    const ipKey = `q:ip:${ip}:${day}`;
    const hostUsed = Number(await store.get(hostKey) || "0");
    const ipUsed = Number(await store.get(ipKey) || "0");
    if (hostUsed >= domainQuota || ipUsed >= ipQuota) {
      throw new HttpError(429, "daily quota exceeded for this domain or network");
    }
    await store.set(hostKey, String(hostUsed + 1), 36 * 3600);
    await store.set(ipKey, String(ipUsed + 1), 36 * 3600);
    const run = {
      id: randomBytes(16).toString("hex"),
      challengeId: id,
      url: claim.u,
      host: claim.h,
      preset: preset.id,
      clientIp: ip,
      status: "running",
      startedAt: new Date(now()).toISOString(),
      maxRps: policy.maxRps,
      durationSeconds: seconds,
      hasBreaking: false,
      breakingRps: 0,
      peakRps: 0,
      peakP95: 0,
      errorRate: 0,
      verdict: "",
      report: null,
      samples: [],
      optIn: body.opt_in === true,
      badgeSvg: badgeSvg("running", "", 0, false),
      originHost: selfHost(request)
    };
    await saveRun(run);
    return json(runView(run), 201);
  }
  function runEvents(id) {
    const encoder = new TextEncoder();
    const stream = new ReadableStream({
      async start(controller) {
        const send = (event, data) => {
          controller.enqueue(encoder.encode(`event: ${event}
data: ${JSON.stringify(data)}

`));
        };
        try {
          let run = await loadRun(id);
          if (!run) {
            controller.enqueue(encoder.encode(`event: done
data: ${JSON.stringify({ status: "missing" })}

`));
            return;
          }
          if (run.status === "running") {
            const locked = await acquire(id);
            if (locked) run = await execute(run, send);
            else run = await poll(id, send);
          } else {
            for (const sample of run.samples) send("sample", sample);
          }
          send("done", { status: run.status, run: runView(run) });
        } catch (err) {
          console.error(err);
          send("done", { status: "failed" });
        } finally {
          controller.close();
        }
      }
    });
    return new Response(stream, {
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache, no-transform",
        "X-Accel-Buffering": "no"
      }
    });
  }
  async function execute(run, send) {
    const page = new URL(run.url);
    const demo = isDemoPath(page, run.originHost);
    const getter = demo ? demoGetter(createGate()) : options.getter ?? externalGetter(policy);
    const preset = presetById(run.preset, run.durationSeconds);
    if (!preset) return finish(run, [], emptyProbe(), false, true);
    let probed = emptyProbe();
    try {
      const result = await probe(run.url, getter);
      probed = result;
    } catch (err) {
      probed = { ...emptyProbe(), error: err instanceof Error ? err.message : "probe failed" };
    }
    const curve = await runCurve({
      url: run.url,
      preset,
      maxRps: run.maxRps,
      durationSeconds: run.durationSeconds,
      policy,
      getter,
      killed,
      onSample: async (sample) => {
        run.samples.push(sample);
        await saveRun(run);
        send("sample", sample);
      }
    });
    return finish(run, curve.samples, probed, curve.cancelled || await killed(), false);
  }
  async function poll(id, send) {
    let sent = 0;
    const deadline = Date.now() + (platformMax + 30) * 1e3;
    let run = await loadRun(id);
    while (run && run.status === "running" && Date.now() < deadline) {
      for (const sample of run.samples.slice(sent)) {
        send("sample", sample);
        sent += 1;
      }
      await sleep(400);
      run = await loadRun(id);
    }
    if (!run) throw new HttpError(404, "run not found");
    for (const sample of run.samples.slice(sent)) send("sample", sample);
    return run;
  }
  async function finish(run, samples, probed, cancelled, failed) {
    const summary = summarize(samples.length ? samples : run.samples);
    const preset = presetById(run.preset, run.durationSeconds);
    const report = template({
      url: run.url,
      presetId: run.preset,
      presetName: preset?.name || run.preset,
      maxRps: run.maxRps,
      summary,
      probe: probed
    });
    run.samples = samples.length ? samples : run.samples;
    run.hasBreaking = summary.hasBreaking;
    run.breakingRps = summary.breakingRps;
    run.peakRps = summary.peakRps;
    run.peakP95 = summary.peakP95;
    run.errorRate = summary.peakError;
    run.verdict = report.verdict;
    run.report = report;
    run.endedAt = new Date(now()).toISOString();
    if (cancelled) run.status = "cancelled";
    else if (failed && summary.requests === 0) {
      run.status = "failed";
      run.error = "load generator failed";
    } else run.status = "completed";
    run.badgeSvg = badgeSvg(run.status, run.verdict, run.peakP95, run.hasBreaking);
    await saveRun(run);
    return run;
  }
  async function getRun(id) {
    const run = await loadRun(id);
    if (!run) throw new HttpError(404, "run not found");
    return json(runView(run));
  }
  async function runBadge(id) {
    const run = await loadRun(id);
    if (!run) return new Response("not found", { status: 404 });
    return new Response(run.badgeSvg, {
      headers: { "Content-Type": "image/svg+xml", "Cache-Control": "no-store" }
    });
  }
  async function setKill(request, on) {
    const got = adminCredential(request);
    if (!adminToken || !safeEqual(got, adminToken)) throw new HttpError(401, "admin token required");
    if (!on && process.env.KILL_SWITCH === "1") throw new HttpError(409, "kill switch is pinned by the environment");
    await store.set("kill", on ? "1" : "0", 30 * 24 * 3600);
    return json({ kill: on });
  }
  async function checkFile(request, claim) {
    const page = new URL(claim.u);
    const file = new URL(page.toString());
    file.pathname = `/.well-known/stampede-${claim.t}.txt`;
    file.search = "";
    file.hash = "";
    normalizeForRequest(file.toString(), request);
    if (normalizeHost(page.hostname) === selfHost(request) && !isDemoPath(page, selfHost(request))) {
      throw new HttpError(400, "on this host, the only site Stampede will test is the demo shop");
    }
    let body = "";
    let status = 0;
    if (isDemoPath(page, selfHost(request))) {
      const demo = demoResult(file);
      body = demo.body.toString("utf8");
      status = demo.status;
    } else if (options.getter) {
      const res = await options.getter(file.toString());
      body = res.body.toString("utf8");
      status = res.status;
    } else {
      const res = await safeGet(file.toString(), policy, { maxBytes: 256, timeoutMs: 8e3 });
      body = res.body.toString("utf8");
      status = res.status;
    }
    if (status !== 200 || !safeEqual(body.trim(), claim.t)) {
      throw new HttpError(422, "ownership file did not match the token");
    }
  }
  async function checkDns(claim) {
    const name = `_stampede.${claim.h}`;
    const want = `stampede-verify=${claim.t}`;
    let records = [];
    try {
      records = options.txtLookup ? await options.txtLookup(name) : await defaultTxt(name);
    } catch {
      throw new HttpError(422, "DNS TXT record did not match the token");
    }
    if (!records.some((record) => safeEqual(record.trim(), want))) {
      throw new HttpError(422, "DNS TXT record did not match the token");
    }
  }
  async function readFlag(id) {
    const raw = await store.get(`ch:${id}`);
    if (!raw) return { verified: false, method: "", grant: "" };
    try {
      const parsed = JSON.parse(raw);
      return { verified: parsed.verified === true, method: parsed.method || "", grant: parsed.grant || "" };
    } catch {
      return { verified: false, method: "", grant: "" };
    }
  }
  async function acquire(id) {
    const key = `runlock:${id}`;
    if (await store.get(key)) return false;
    await store.set(key, "1", 180);
    return true;
  }
  async function saveRun(run) {
    await store.set(`run:${run.id}`, JSON.stringify(run), 7 * 24 * 3600);
  }
  async function loadRun(id) {
    const raw = await store.get(`run:${id}`);
    if (!raw) return null;
    try {
      return JSON.parse(raw);
    } catch {
      return null;
    }
  }
  return { handle, store };
}
function challengeView(id, claim, verified, method) {
  const page = new URL(claim.u);
  const file = new URL(page.toString());
  file.pathname = `/.well-known/stampede-${claim.t}.txt`;
  file.search = "";
  file.hash = "";
  return {
    id,
    url: claim.u,
    host: claim.h,
    token: claim.t,
    file_url: file.toString(),
    file_path: file.pathname,
    dns_name: `_stampede.${claim.h}`,
    dns_value: `stampede-verify=${claim.t}`,
    expires_at: new Date(claim.e).toISOString(),
    verified,
    method
  };
}
function runView(run) {
  return {
    id: run.id,
    url: run.url,
    host: run.host,
    preset: run.preset,
    status: run.status,
    started_at: run.startedAt,
    ended_at: run.endedAt,
    max_rps: run.maxRps,
    duration_seconds: run.durationSeconds,
    has_breaking: run.hasBreaking,
    breaking_rps: run.breakingRps,
    peak_rps: run.peakRps,
    peak_p95_ms: run.peakP95,
    error_rate: run.errorRate,
    verdict: run.verdict,
    report: run.report,
    samples: run.samples,
    error: run.error || "",
    badge_path: `/v1/runs/${run.id}/badge.svg`,
    badge_svg: run.badgeSvg,
    result_path: `/r/${run.id}`
  };
}
function emptyProbe() {
  return { pageMs: 0, pageBytes: 0, pageStatus: 0, assets: [] };
}
function assertOwnHostPath(url, selfHost) {
  if (normalizeHost(url.hostname) !== normalizeHost(selfHost)) return;
  if (url.pathname === "/demo" || url.pathname === "/demo/") return;
  throw new PolicyError("on this host, the only site Stampede will test is the demo shop");
}
function clientIp(request, trust) {
  if (trust) {
    const xff = request.headers.get("x-forwarded-for");
    if (xff) return xff.split(",")[0]?.trim().slice(0, 64) || "unknown";
  }
  return request.headers.get("x-real-ip")?.slice(0, 64) || "local";
}
function adminCredential(request) {
  const header2 = request.headers.get("x-stampede-admin");
  if (header2?.trim()) return header2.trim();
  const auth = request.headers.get("authorization") || "";
  if (auth.toLowerCase().startsWith("bearer ")) return auth.slice(7).trim();
  return "";
}
var HttpError = class extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
    this.name = "HttpError";
  }
  status;
};
async function readJson(request) {
  const text = await request.text();
  if (!text || text.length > 1e6) throw new HttpError(400, "invalid JSON body");
  try {
    const parsed = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("shape");
    return parsed;
  } catch {
    throw new HttpError(400, "invalid JSON body");
  }
}
function json(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "Cache-Control": "no-store" }
  });
}
function secret() {
  return process.env.STAMPEDE_SECRET || "stampede-public-demo";
}
function sign(claim) {
  const body = Buffer.from(JSON.stringify(claim)).toString("base64url");
  return `v1.${body}.${hmac(body)}`;
}
function readClaim(id, nowMs) {
  const parts = id.split(".");
  if (parts.length !== 3 || parts[0] !== "v1" || !parts[1] || !parts[2] || !safeEqual(hmac(parts[1]), parts[2])) {
    throw new HttpError(404, "challenge not found");
  }
  let claim;
  try {
    claim = JSON.parse(Buffer.from(parts[1], "base64url").toString("utf8"));
  } catch {
    throw new HttpError(404, "challenge not found");
  }
  if (!claim.u || !claim.t || !claim.e || claim.e < nowMs) throw new HttpError(410, "verification challenge expired");
  return claim;
}
function signGrant(grant) {
  const body = Buffer.from(JSON.stringify(grant)).toString("base64url");
  return `g1.${body}.${hmac(body)}`;
}
function readGrant(token, nowMs) {
  const parts = token.split(".");
  if (parts.length !== 3 || parts[0] !== "g1" || !parts[1] || !parts[2] || !safeEqual(hmac(parts[1]), parts[2])) return null;
  try {
    const grant = JSON.parse(Buffer.from(parts[1], "base64url").toString("utf8"));
    if (!grant.id || !grant.u || grant.e < nowMs) return null;
    return grant;
  } catch {
    return null;
  }
}
function hmac(body) {
  return createHmac("sha256", secret()).update(body).digest("base64url");
}
function safeEqual(got, want) {
  const a = Buffer.from(got);
  const b = Buffer.from(want);
  if (a.length !== b.length || a.length === 0) return false;
  return timingSafeEqual(a, b);
}
async function defaultTxt(name) {
  const resolver = new Resolver();
  const rows = await resolver.resolveTxt(name);
  return rows.map((parts) => parts.join(""));
}

var app = createApp();
var config = {
  maxDuration: 90
};
async function handler(req, res) {
  try {
    req.url = v1Url(req.url || "/");
    const request = await toRequest(req, "https");
    const response = await app.handle(request);
    await writeResponse(res, response);
  } catch (err) {
    console.error(err);
    if (!res.headersSent) {
      res.statusCode = 500;
      res.setHeader("Content-Type", "application/json");
    }
    res.end(JSON.stringify({ error: "internal error" }));
  }
}
function v1Url(raw) {
  const url = new URL(raw || "/", "https://localhost");
  if (url.pathname !== "/api/v1" && url.pathname !== "/v1") return raw || "/";
  if (!url.searchParams.has("path")) return raw || "/";
  const routed = url.searchParams.get("path") || "";
  url.searchParams.delete("path");
  const suffix = routed.replace(/^\/+/, "");
  url.pathname = suffix ? `/v1/${suffix}` : "/v1";
  return `${url.pathname}${url.search}`;
}
export {
  config,
  handler as default,
  v1Url
};
