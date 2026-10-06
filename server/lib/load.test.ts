import assert from "node:assert/strict";
import test from "node:test";
import { runCurve } from "./load";
import { summarize } from "./metrics";
import { top5 } from "./preset";
import { defaultPolicy } from "./safety";

test("curve is GET-only, ramps workers, and records a breaking point", async () => {
  const methods: string[] = [];
  let n = 0;
  const result = await runCurve({
    url: "https://example.com/",
    preset: top5(2),
    maxRps: 20,
    durationSeconds: 2,
    policy: defaultPolicy({ maxRps: 20 }),
    killed: () => false,
    getter: async () => {
      methods.push("GET");
      n += 1;
      if (n > 12) return { status: 503, body: Buffer.from("no"), ms: 30, contentType: "text/plain", finalUrl: "https://example.com/" };
      return { status: 200, body: Buffer.from("ok"), ms: 15, contentType: "text/html", finalUrl: "https://example.com/" };
    },
  });
  assert.equal(result.samples.length, 2);
  assert.equal(result.samples[0]?.workers, 10);
  assert.ok((result.samples[1]?.workers || 0) >= 45);
  assert.ok(methods.every((method) => method === "GET"));
  const summary = summarize(result.samples);
  assert.equal(summary.hasBreaking, true);
});
