import { readFile, readdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as esbuild from "esbuild";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const apiDir = path.join(root, "api");

// Drop anything previously emitted under api/ so a catch-all directory cannot
// linger and get counted as extra functions.
await rm(apiDir, { recursive: true, force: true });

const shared = {
  bundle: true,
  platform: "node",
  format: "esm",
  target: "node22",
  packages: "bundle",
  // Runtime Cache is provided by the Vercel Node environment, not inlined.
  external: ["@vercel/functions"],
  sourcemap: false,
  legalComments: "none",
  logLevel: "info",
};

const entries = [
  ["server/handlers/shop.ts", "api/shop.js"],
  ["server/handlers/wellknown.ts", "api/wellknown.js"],
  ["server/handlers/v1.ts", "api/v1.js"],
];

for (const [entry, outfile] of entries) {
  await esbuild.build({
    ...shared,
    absWorkingDir: root,
    entryPoints: [entry],
    outfile,
  });
  const full = path.join(root, outfile);
  const cleaned = (await readFile(full, "utf8")).replace(/^\/\/ server\/.*\n/gm, "");
  if (cleaned.includes("server/lib") || /from\s+["']\.\.\//.test(cleaned)) {
    throw new Error(`${outfile} still references server/lib or a parent import`);
  }
  await writeFile(full, cleaned);
}

const files = (await readdir(apiDir, { recursive: true })).filter((name) => name.endsWith(".js") || name.endsWith(".ts"));
const expected = ["shop.js", "v1.js", "wellknown.js"];
const listed = files.map((name) => name.split(path.sep).join("/")).sort();
if (listed.join(",") !== expected.join(",")) {
  throw new Error(`api/ must contain exactly ${expected.join(", ")}; found ${listed.join(", ") || "(none)"}`);
}

console.log("bundled", listed.join(", "));
