import { createHmac, randomBytes, timingSafeEqual } from "node:crypto";
import { Resolver } from "node:dns/promises";
import { badgeSvg } from "./badge";
import { createGate, demoResult, isDemoPath, isLaunchDemoTarget, sleep } from "./demo";
import {
  CHALLENGE_LIMIT_PER_HOUR,
  demoDomainQuota,
  domainQuota,
  IP_QUOTA,
  MAX_WORKERS,
  SAFETY_MAX_SECONDS,
  allowHosts,
  defaultDurationSeconds,
  maxRps,
  platformMaxSeconds,
} from "./limits";
import { demoGetter, externalGetter, probe, runCurve, type Getter } from "./load";
import type { Sample } from "./metrics";
import { summarize } from "./metrics";
import { allPresets, presetById } from "./preset";
import { template, type Probe, type Report } from "./report";
import { PolicyError, defaultPolicy, normalize, normalizeHost, safeGet, validatePlan, type Policy } from "./safety";
import { memoryStore, runtimeStore, type Store } from "./store";

type Claim = { u: string; h: string; t: string; e: number; ip: string };
type Grant = { id: string; u: string; h: string; t: string; m: string; e: number };

type RunRecord = {
  id: string;
  challengeId: string;
  url: string;
  host: string;
  preset: string;
  clientIp: string;
  status: "running" | "completed" | "failed" | "cancelled";
  startedAt: string;
  endedAt?: string;
  maxRps: number;
  durationSeconds: number;
  hasBreaking: boolean;
  breakingRps: number;
  peakRps: number;
  peakP95: number;
  errorRate: number;
  verdict: string;
  report: Report | null;
  samples: Sample[];
  error?: string;
  optIn: boolean;
  badgeSvg: string;
  originHost: string;
};

export type AppOptions = {
  store?: Store;
  trustProxy?: boolean;
  policy?: Policy;
  platformMaxSeconds?: number;
  defaultDuration?: number;
  domainQuota?: number;
  demoDomainQuota?: number;
  ipQuota?: number;
  adminToken?: string;
  now?: () => number;
  getter?: Getter;
  txtLookup?: (name: string) => Promise<string[]>;
};

export type App = {
  handle: (request: Request) => Promise<Response>;
  store: Store;
};

export function createApp(options: AppOptions = {}): App {
  const store = options.store ?? runtimeStore();
  const policy = options.policy ?? defaultPolicy({ allowHosts: allowHosts(), maxRps: maxRps() });
  const platformMax = options.platformMaxSeconds ?? platformMaxSeconds();
  const presetDuration = options.defaultDuration ?? defaultDurationSeconds();
  const domainQuotaLimit = options.domainQuota ?? domainQuota();
  const demoDomainQuotaLimit = options.demoDomainQuota ?? demoDomainQuota();
  const ipQuota = options.ipQuota ?? IP_QUOTA;
  const adminToken = options.adminToken ?? process.env.STAMPEDE_ADMIN_TOKEN ?? "";
  const now = options.now ?? (() => Date.now());
  const trustProxy = options.trustProxy ?? (process.env.VERCEL === "1" || process.env.TRUST_PROXY === "1");

  async function handle(request: Request): Promise<Response> {
    try {
      const url = new URL(request.url);
      const path = url.pathname.replace(/^\/api(?=\/)/, "") || "/";
      if (request.method === "GET" && (path === "/healthz" || path === "/v1/healthz")) {
        return json({ status: "ok" });
      }
      if (request.method === "GET" && path === "/v1/config") return await config(request);
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

  function selfHost(request: Request): string {
    return normalizeHost(new URL(request.url).hostname);
  }

  // The demo shop is this deployment. Allow its own hostname, including localhost,
  // without opening other private targets. Metadata hosts stay blocked.
  function normalizeForRequest(raw: string, request: Request) {
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

  function demoUrl(request: Request): string {
    const url = new URL(request.url);
    return `${url.protocol}//${url.host}/demo`;
  }

  async function killed(): Promise<boolean> {
    if (process.env.KILL_SWITCH === "1") return true;
    return (await store.get("kill")) === "1";
  }

  async function config(request: Request): Promise<Response> {
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
      domain_quota: domainQuotaLimit,
      demo_domain_quota: Math.max(domainQuotaLimit, demoDomainQuotaLimit),
      ip_quota: ipQuota,
    });
  }

  function presets(): Response {
    return json({
      presets: allPresets(presetDuration).map((p) => ({
        id: p.id,
        name: p.name,
        blurb: p.blurb,
        assumptions: p.assumptions,
        duration_seconds: p.durationSeconds,
        start_workers: p.startWorkers,
        end_workers: p.endWorkers,
        peak_fraction: p.peak,
      })),
    });
  }

  async function createChallenge(request: Request): Promise<Response> {
    const body = await readJson(request);
    const raw = typeof body.url === "string" ? body.url : "";
    const target = normalizeForRequest(raw, request);
    assertOwnHostPath(target.url, selfHost(request));
    const ip = clientIp(request, trustProxy);
    const hour = new Date(now()).toISOString().slice(0, 13);
    const rateKey = `chrate:${ip}:${hour}`;
    const used = Number((await store.get(rateKey)) || "0");
    if (used >= CHALLENGE_LIMIT_PER_HOUR) {
      throw new HttpError(429, "too many verification challenges from this network");
    }
    await store.set(rateKey, String(used + 1), 3700);
    const claim: Claim = {
      u: target.url.toString(),
      h: target.host,
      t: `st_${randomBytes(16).toString("hex")}`,
      e: now() + 30 * 60 * 1000,
      ip,
    };
    const id = sign(claim);
    await store.set(`ch:${id}`, JSON.stringify({ verified: false, method: "" }), 30 * 60);
    return json(challengeView(id, claim, false, ""), 201);
  }

  async function getChallenge(id: string): Promise<Response> {
    const claim = readClaim(id, now());
    const saved = await readFlag(id);
    return json(challengeView(id, claim, saved.verified, saved.method));
  }

  async function verifyChallenge(request: Request, id: string): Promise<Response> {
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
      e: now() + 24 * 60 * 60 * 1000,
    });
    await store.set(`ch:${id}`, JSON.stringify({ verified: true, method, grant }), 24 * 60 * 60);
    return json({ verified: true, method, grant });
  }

  async function createRun(request: Request): Promise<Response> {
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
    const allowed = saved.verified || (grant != null && grant.id === id && grant.u === claim.u);
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
    const hostUsed = Number((await store.get(hostKey)) || "0");
    const ipUsed = Number((await store.get(ipKey)) || "0");
    const hostQuota = isLaunchDemoTarget(page, selfHost(request))
      ? Math.max(domainQuotaLimit, demoDomainQuotaLimit)
      : domainQuotaLimit;
    if (hostUsed >= hostQuota || ipUsed >= ipQuota) {
      throw new HttpError(429, "daily quota exceeded for this domain or network");
    }
    await store.set(hostKey, String(hostUsed + 1), 36 * 3600);
    await store.set(ipKey, String(ipUsed + 1), 36 * 3600);
    const run: RunRecord = {
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
      originHost: selfHost(request),
    };
    await saveRun(run);
    return json(runView(run), 201);
  }

  function runEvents(id: string): Response {
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      async start(controller) {
        const send = (event: string, data: unknown) => {
          controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
        };
        try {
          let run = await loadRun(id);
          if (!run) {
            controller.enqueue(encoder.encode(`event: done\ndata: ${JSON.stringify({ status: "missing" })}\n\n`));
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
      },
    });
    return new Response(stream, {
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache, no-transform",
        "X-Accel-Buffering": "no",
      },
    });
  }

  async function execute(run: RunRecord, send: (event: string, data: unknown) => void): Promise<RunRecord> {
    const page = new URL(run.url);
    const demo = isDemoPath(page, run.originHost);
    const getter = demo ? demoGetter(createGate()) : options.getter ?? externalGetter(policy);
    const preset = presetById(run.preset, run.durationSeconds);
    if (!preset) return finish(run, [], emptyProbe(), false, true);
    let probed: Probe = emptyProbe();
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
      },
    });
    return finish(run, curve.samples, probed, curve.cancelled || (await killed()), false);
  }

  async function poll(id: string, send: (event: string, data: unknown) => void): Promise<RunRecord> {
    let sent = 0;
    const deadline = Date.now() + (platformMax + 30) * 1000;
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

  async function finish(run: RunRecord, samples: Sample[], probed: Probe, cancelled: boolean, failed: boolean): Promise<RunRecord> {
    const summary = summarize(samples.length ? samples : run.samples);
    const preset = presetById(run.preset, run.durationSeconds);
    const report = template({
      url: run.url,
      presetId: run.preset,
      presetName: preset?.name || run.preset,
      maxRps: run.maxRps,
      summary,
      probe: probed,
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

  async function getRun(id: string): Promise<Response> {
    const run = await loadRun(id);
    if (!run) throw new HttpError(404, "run not found");
    return json(runView(run));
  }

  async function runBadge(id: string): Promise<Response> {
    const run = await loadRun(id);
    if (!run) return new Response("not found", { status: 404 });
    return new Response(run.badgeSvg, {
      headers: { "Content-Type": "image/svg+xml", "Cache-Control": "no-store" },
    });
  }

  async function setKill(request: Request, on: boolean): Promise<Response> {
    const got = adminCredential(request);
    if (!adminToken || !safeEqual(got, adminToken)) throw new HttpError(401, "admin token required");
    if (!on && process.env.KILL_SWITCH === "1") throw new HttpError(409, "kill switch is pinned by the environment");
    await store.set("kill", on ? "1" : "0", 30 * 24 * 3600);
    return json({ kill: on });
  }

  async function checkFile(request: Request, claim: Claim): Promise<void> {
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
      const res = await safeGet(file.toString(), policy, { maxBytes: 256, timeoutMs: 8000 });
      body = res.body.toString("utf8");
      status = res.status;
    }
    if (status !== 200 || !safeEqual(body.trim(), claim.t)) {
      throw new HttpError(422, "ownership file did not match the token");
    }
  }

  async function checkDns(claim: Claim): Promise<void> {
    const name = `_stampede.${claim.h}`;
    const want = `stampede-verify=${claim.t}`;
    let records: string[] = [];
    try {
      records = options.txtLookup ? await options.txtLookup(name) : await defaultTxt(name);
    } catch {
      throw new HttpError(422, "DNS TXT record did not match the token");
    }
    if (!records.some((record) => safeEqual(record.trim(), want))) {
      throw new HttpError(422, "DNS TXT record did not match the token");
    }
  }

  async function readFlag(id: string): Promise<{ verified: boolean; method: string; grant: string }> {
    const raw = await store.get(`ch:${id}`);
    if (!raw) return { verified: false, method: "", grant: "" };
    try {
      const parsed = JSON.parse(raw) as { verified?: boolean; method?: string; grant?: string };
      return { verified: parsed.verified === true, method: parsed.method || "", grant: parsed.grant || "" };
    } catch {
      return { verified: false, method: "", grant: "" };
    }
  }

  async function acquire(id: string): Promise<boolean> {
    const key = `runlock:${id}`;
    if (await store.get(key)) return false;
    await store.set(key, "1", 180);
    return true;
  }

  async function saveRun(run: RunRecord): Promise<void> {
    await store.set(`run:${run.id}`, JSON.stringify(run), 7 * 24 * 3600);
  }

  async function loadRun(id: string): Promise<RunRecord | null> {
    const raw = await store.get(`run:${id}`);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as RunRecord;
    } catch {
      return null;
    }
  }

  return { handle, store };
}

function challengeView(id: string, claim: Claim, verified: boolean, method: string) {
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
    method,
  };
}

function runView(run: RunRecord) {
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
    result_path: `/r/${run.id}`,
  };
}

function emptyProbe(): Probe {
  return { pageMs: 0, pageBytes: 0, pageStatus: 0, assets: [] };
}

function assertOwnHostPath(url: URL, selfHost: string): void {
  if (normalizeHost(url.hostname) !== normalizeHost(selfHost)) return;
  if (url.pathname === "/demo" || url.pathname === "/demo/") return;
  throw new PolicyError("on this host, the only site Stampede will test is the demo shop");
}

function clientIp(request: Request, trust: boolean): string {
  if (trust) {
    const xff = request.headers.get("x-forwarded-for");
    if (xff) return xff.split(",")[0]?.trim().slice(0, 64) || "unknown";
  }
  return request.headers.get("x-real-ip")?.slice(0, 64) || "local";
}

function adminCredential(request: Request): string {
  const header = request.headers.get("x-stampede-admin");
  if (header?.trim()) return header.trim();
  const auth = request.headers.get("authorization") || "";
  if (auth.toLowerCase().startsWith("bearer ")) return auth.slice(7).trim();
  return "";
}

class HttpError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "HttpError";
  }
}

async function readJson(request: Request): Promise<Record<string, unknown>> {
  const text = await request.text();
  if (!text || text.length > 1_000_000) throw new HttpError(400, "invalid JSON body");
  try {
    const parsed = JSON.parse(text) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("shape");
    return parsed as Record<string, unknown>;
  } catch {
    throw new HttpError(400, "invalid JSON body");
  }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
  });
}

function secret(): string {
  return process.env.STAMPEDE_SECRET || "stampede-public-demo";
}

function sign(claim: Claim): string {
  const body = Buffer.from(JSON.stringify(claim)).toString("base64url");
  return `v1.${body}.${hmac(body)}`;
}

function readClaim(id: string, nowMs: number): Claim {
  const parts = id.split(".");
  if (parts.length !== 3 || parts[0] !== "v1" || !parts[1] || !parts[2] || !safeEqual(hmac(parts[1]), parts[2])) {
    throw new HttpError(404, "challenge not found");
  }
  let claim: Claim;
  try {
    claim = JSON.parse(Buffer.from(parts[1], "base64url").toString("utf8")) as Claim;
  } catch {
    throw new HttpError(404, "challenge not found");
  }
  if (!claim.u || !claim.t || !claim.e || claim.e < nowMs) throw new HttpError(410, "verification challenge expired");
  return claim;
}

function signGrant(grant: Grant): string {
  const body = Buffer.from(JSON.stringify(grant)).toString("base64url");
  return `g1.${body}.${hmac(body)}`;
}

function readGrant(token: string, nowMs: number): Grant | null {
  const parts = token.split(".");
  if (parts.length !== 3 || parts[0] !== "g1" || !parts[1] || !parts[2] || !safeEqual(hmac(parts[1]), parts[2])) return null;
  try {
    const grant = JSON.parse(Buffer.from(parts[1], "base64url").toString("utf8")) as Grant;
    if (!grant.id || !grant.u || grant.e < nowMs) return null;
    return grant;
  } catch {
    return null;
  }
}

function hmac(body: string): string {
  return createHmac("sha256", secret()).update(body).digest("base64url");
}

function safeEqual(got: string, want: string): boolean {
  const a = Buffer.from(got);
  const b = Buffer.from(want);
  if (a.length !== b.length || a.length === 0) return false;
  return timingSafeEqual(a, b);
}

async function defaultTxt(name: string): Promise<string[]> {
  const resolver = new Resolver();
  const rows = await resolver.resolveTxt(name);
  return rows.map((parts) => parts.join(""));
}

export function renderDemo(url: URL): { status: number; body: Buffer; contentType: string; delayMs: number } {
  return demoResult(url);
}
