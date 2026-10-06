import assert from "node:assert/strict";
import test from "node:test";
import { createApp } from "./server";
import { memoryStore } from "./store";
import { defaultPolicy } from "./safety";

const origin = "https://stampede.test";

function app(extra: Parameters<typeof createApp>[0] = {}) {
  return createApp({
    store: memoryStore(),
    trustProxy: true,
    policy: defaultPolicy({ maxRps: 8, maxDurationSeconds: 180 }),
    platformMaxSeconds: 40,
    defaultDuration: 30,
    adminToken: "admin-token-test",
    ...extra,
  });
}

async function post(handle: (req: Request) => Promise<Response>, path: string, body: unknown, ip = "203.0.113.9") {
  const res = await handle(
    new Request(`${origin}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Forwarded-For": ip },
      body: JSON.stringify(body),
    }),
  );
  const json = (await res.json()) as Record<string, unknown>;
  return { status: res.status, json };
}

test("demo loop requires ownership, then streams a report and badge", async () => {
  const { handle } = app();
  const config = await handle(new Request(`${origin}/v1/config`));
  const cfg = (await config.json()) as { demo_url: string; platform: string; scheduler: boolean; model: string };
  assert.equal(cfg.demo_url, `${origin}/demo`);
  assert.equal(cfg.platform, "vercel");
  assert.equal(cfg.scheduler, false);
  assert.equal(cfg.model, "template");

  const blocked = await post(handle, "/v1/challenges", { url: "http://169.254.169.254/" });
  assert.equal(blocked.status, 400);
  const self = await post(handle, "/v1/challenges", { url: `${origin}/api/v1/runs` });
  assert.equal(self.status, 400);

  const created = await post(handle, "/v1/challenges", { url: `${origin}/demo` });
  assert.equal(created.status, 201);
  const id = String(created.json.id);
  assert.match(String(created.json.token), /^st_/);
  assert.match(String(created.json.file_path), /^\/\.well-known\/stampede-/);

  const denied = await post(handle, "/v1/runs", { challenge_id: id, preset: "top5", duration_seconds: 2 });
  assert.equal(denied.status, 403);

  const verified = await post(handle, `/v1/challenges/${encodeURIComponent(id)}/verify`, { method: "file" });
  assert.equal(verified.status, 200);
  assert.equal(verified.json.verified, true);

  const tooLong = await post(handle, "/v1/runs", { challenge_id: id, preset: "top5", duration_seconds: 120 });
  assert.equal(tooLong.status, 400);

  const started = await post(handle, "/v1/runs", {
    challenge_id: id,
    preset: "top5",
    duration_seconds: 2,
    grant: verified.json.grant,
  });
  assert.equal(started.status, 201);
  const runId = String(started.json.id);
  const stream = await handle(new Request(`${origin}/v1/runs/${runId}/events`));
  const text = await stream.text();
  assert.match(text, /event: sample/);
  assert.match(text, /event: done/);
  const done = text.split("event: done\n")[1] || "";
  const payload = JSON.parse(done.replace(/^data: /, "").trim()) as { run: { status: string; report: { model: string; cost: { note: string } }; samples: { workers: number }[]; badge_svg: string } };
  assert.equal(payload.run.status, "completed");
  assert.equal(payload.run.report.model, "template");
  assert.match(payload.run.report.cost.note, /Vercel/);
  assert.equal(payload.run.samples[0]?.workers, 10);
  assert.ok((payload.run.samples.at(-1)?.workers || 0) >= 45);
  assert.match(payload.run.badge_svg, /<svg/);

  const badge = await handle(new Request(`${origin}/v1/runs/${runId}/badge.svg`));
  assert.equal(badge.headers.get("content-type"), "image/svg+xml");
  const svg = await badge.text();
  assert.match(svg, /Launch-ready|Needs work/);
});

test("the deployment's own loopback demo is allowed and other loopback paths are not", async () => {
  const { handle } = app();
  const created = await handle(
    new Request("http://127.0.0.1:8787/v1/challenges", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url: "http://127.0.0.1:8787/demo" }),
    }),
  );
  assert.equal(created.status, 201);
  const other = await handle(
    new Request("http://127.0.0.1:8787/v1/challenges", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url: "http://127.0.0.1:9/" }),
    }),
  );
  assert.equal(other.status, 400);
});

test("quotas, kill switch, and external ownership stay enforced", async () => {
  const files = new Map<string, string>();
  const { handle } = app({
    domainQuota: 1,
    ipQuota: 5,
    getter: async (url) => {
      const token = files.get(url);
      if (!token) return { status: 404, body: Buffer.from("missing"), ms: 5, contentType: "text/plain", finalUrl: url };
      return { status: 200, body: Buffer.from(token), ms: 5, contentType: "text/plain", finalUrl: url };
    },
  });
  const created = await post(handle, "/v1/challenges", { url: "https://example.com/" });
  assert.equal(created.status, 201);
  const id = String(created.json.id);
  const missing = await post(handle, `/v1/challenges/${encodeURIComponent(id)}/verify`, { method: "file" });
  assert.equal(missing.status, 422);
  files.set(String(created.json.file_url), String(created.json.token));
  const verified = await post(handle, `/v1/challenges/${encodeURIComponent(id)}/verify`, { method: "file" });
  assert.equal(verified.status, 200);

  const first = await post(handle, "/v1/runs", { challenge_id: id, preset: "top5", duration_seconds: 1 });
  assert.equal(first.status, 201);
  const second = await post(handle, "/v1/runs", { challenge_id: id, preset: "top5", duration_seconds: 1 });
  assert.equal(second.status, 429);

  const paused = await handle(
    new Request(`${origin}/v1/admin/kill`, { method: "POST", headers: { Authorization: "Bearer admin-token-test" } }),
  );
  assert.equal(paused.status, 200);
  const fresh = await post(handle, "/v1/challenges", { url: "https://example.org/" }, "203.0.113.10");
  files.set(String(fresh.json.file_url), String(fresh.json.token));
  await post(handle, `/v1/challenges/${encodeURIComponent(String(fresh.json.id))}/verify`, { method: "file" }, "203.0.113.10");
  const refused = await post(handle, "/v1/runs", { challenge_id: fresh.json.id, preset: "top5" }, "203.0.113.10");
  assert.equal(refused.status, 409);
  const anon = await handle(new Request(`${origin}/v1/admin/resume`, { method: "POST" }));
  assert.equal(anon.status, 401);
});
