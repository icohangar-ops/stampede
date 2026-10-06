import assert from "node:assert/strict";
import test from "node:test";
import { estimate, template } from "./report";

const emptyProbe = { pageMs: 0, pageBytes: 0, pageStatus: 200, assets: [] };

test("template verdicts and Vercel cost note", () => {
  const degraded = template({
    url: "https://example.com/",
    presetId: "numberone",
    presetName: "#1 Product of the Day",
    maxRps: 40,
    summary: { hasBreaking: true, breakingRps: 32, peakRps: 40, peakP95: 1600, peakError: 0.08, requests: 10, errors: 1, breakReason: "latency" },
    probe: {
      pageMs: 120,
      pageBytes: 100,
      pageStatus: 200,
      assets: [
        { url: "https://example.com/hero.png", bytes: 180000, ms: 40, contentType: "image/png", status: 200 },
        { url: "https://example.com/api/pricing", bytes: 20, ms: 900, contentType: "application/json", status: 200 },
      ],
    },
  });
  assert.match(degraded.headline, /32/);
  assert.equal(degraded.verdict, "degraded");
  assert.match(degraded.cost.note.toLowerCase(), /not an invoice/);
  assert.match(degraded.cost.note, /Vercel/);
  assert.ok(degraded.fixes.some((fix) => /image/i.test(fix.title)));

  const fail = template({
    url: "https://example.com/",
    presetId: "top5",
    presetName: "Top 5 of the Day",
    maxRps: 40,
    summary: { hasBreaking: true, breakingRps: 12, peakRps: 20, peakP95: 2000, peakError: 0.4, requests: 4, errors: 1 },
    probe: emptyProbe,
  });
  assert.equal(fail.verdict, "fails");
  assert.match(fail.headline, /Won't survive/);

  const ready = template({
    url: "https://example.com/",
    presetId: "top5",
    presetName: "Top 5 of the Day",
    maxRps: 40,
    summary: { hasBreaking: false, breakingRps: 0, peakRps: 30, peakP95: 80, peakError: 0, requests: 30, errors: 0 },
    probe: emptyProbe,
  });
  assert.equal(ready.verdict, "launch_ready");
  assert.equal(ready.model, "template");
});

test("estimate stays a small positive planning figure", () => {
  const cost = estimate(40, 200);
  assert.equal(cost.instances, 1);
  assert.equal(cost.requests, 576000);
  assert.ok(cost.usd > 1 && cost.usd < 4);
});
