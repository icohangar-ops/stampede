import assert from "node:assert/strict";
import test from "node:test";
import { allPresets, fraction, numberOne, top5, workersAt } from "./preset";

test("preset peaks and worker ramp", () => {
  const top = top5(30);
  const one = numberOne(30);
  let topPeak = 0;
  let onePeak = 0;
  for (let i = 0; i <= 100; i++) {
    const f = i / 100;
    topPeak = Math.max(topPeak, top.intensity(f));
    onePeak = Math.max(onePeak, one.intensity(f));
  }
  assert.ok(topPeak > 0.74 && topPeak < 0.76);
  assert.ok(onePeak > 0.99);
  assert.equal(workersAt(top, 0), 10);
  assert.equal(workersAt(top, 1), 50);
  assert.equal(fraction(0, 1), 1);
  assert.equal(allPresets(30).map((p) => p.id).join(","), "top5,numberone");
  assert.ok(top.assumptions.some((line) => line.includes("Vercel")));
});
