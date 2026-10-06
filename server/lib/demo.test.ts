import assert from "node:assert/strict";
import test from "node:test";
import { createGate, demoResult, isLaunchDemoTarget } from "./demo";

test("demo shop echoes the ownership token and folds as the ramp climbs", () => {
  const file = demoResult(new URL("https://stampede.test/.well-known/stampede-st_abc123.txt"));
  assert.equal(file.status, 200);
  assert.equal(file.body.toString(), "st_abc123");

  const gate = createGate();
  const early = gate.hit();
  assert.equal(early.fail, false);
  assert.ok(early.delayMs < 100);
  const busy = createGate();
  for (let i = 0; i < 23; i++) busy.hit();
  const slow = busy.hit();
  assert.equal(slow.fail, false);
  assert.ok(slow.delayMs >= 1500);
  const hot = createGate();
  for (let i = 0; i < 33; i++) hot.hit();
  const down = hot.hit();
  assert.equal(down.fail, true);

  assert.equal(isLaunchDemoTarget(new URL("https://stampede.test/demo"), "stampede.test"), true);
  assert.equal(isLaunchDemoTarget(new URL("https://stampede-three.vercel.app/demo"), "localhost"), true);
  assert.equal(isLaunchDemoTarget(new URL("https://stampede-three.vercel.app/demo/"), "localhost"), true);
  assert.equal(isLaunchDemoTarget(new URL("https://example.com/demo"), "stampede.test"), false);
  assert.equal(isLaunchDemoTarget(new URL("https://stampede-three.vercel.app/"), "stampede-three.vercel.app"), false);
});
