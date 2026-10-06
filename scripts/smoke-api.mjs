import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const apiDir = path.join(root, "api");
const files = readdirSync(apiDir, { recursive: true })
  .map((name) => String(name).split(path.sep).join("/"))
  .filter((name) => name.endsWith(".js") || name.endsWith(".ts"))
  .sort();
assert.deepEqual(files, ["shop.js", "v1.js", "wellknown.js"]);

for (const name of files) {
  const text = readFileSync(path.join(apiDir, name), "utf8");
  assert.equal(/from\s+["']\.\.?\/server\/lib/.test(text), false, `${name} imports server/lib at runtime`);
  assert.equal(/from\s+["']\.\.\//.test(text), false, `${name} has a relative parent import`);
  assert.equal(text.includes("/var/task/server"), false, `${name} points at /var/task/server`);
}

const shop = (await import("../api/shop.js")).default;
const wellknown = (await import("../api/wellknown.js")).default;
const v1 = (await import("../api/v1.js")).default;

const server = createServer(async (req, res) => {
  const url = new URL(req.url || "/", "http://127.0.0.1");
  if (url.pathname === "/api/shop" || url.pathname === "/demo" || url.pathname.startsWith("/demo/")) {
    if (url.pathname.startsWith("/demo/") && !url.searchParams.has("path")) {
      req.url = `/api/shop?path=${encodeURIComponent(url.pathname.slice("/demo/".length))}`;
    } else if (url.pathname === "/demo") {
      req.url = "/api/shop";
    }
    return shop(req, res);
  }
  if (url.pathname === "/api/wellknown" || url.pathname.startsWith("/.well-known/")) {
    if (url.pathname.startsWith("/.well-known/") && !url.searchParams.has("token")) {
      const token = url.pathname.slice("/.well-known/stampede-".length, -".txt".length);
      req.url = `/api/wellknown?token=${encodeURIComponent(token)}`;
    }
    return wellknown(req, res);
  }
  if (url.pathname === "/healthz") {
    req.url = "/api/v1?path=healthz";
    return v1(req, res);
  }
  if (url.pathname === "/api/v1" || url.pathname.startsWith("/v1/") || url.pathname.startsWith("/api/v1/")) {
    if (url.pathname.startsWith("/v1/") || url.pathname.startsWith("/api/v1/")) {
      const rest = url.pathname.replace(/^\/api\/v1\/|^\/v1\//, "");
      req.url = `/api/v1?path=${rest}`;
    }
    return v1(req, res);
  }
  res.statusCode = 404;
  res.end("not found");
});

await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const { port } = server.address();
const base = `http://127.0.0.1:${port}`;

try {
  const health = await fetch(`${base}/api/v1?path=healthz`);
  assert.equal(health.status, 200);
  assert.match(health.headers.get("content-type") || "", /^application\/json/);
  assert.deepEqual(await health.json(), { status: "ok" });

  const healthRewrite = await fetch(`${base}/healthz`);
  assert.equal(healthRewrite.status, 200);
  assert.deepEqual(await healthRewrite.json(), { status: "ok" });

  const nested = await fetch(`${base}/v1/config`);
  assert.equal(nested.status, 200);
  const config = await nested.json();
  assert.equal(config.platform, "vercel");

  const shopRes = await fetch(`${base}/demo`);
  assert.equal(shopRes.status, 200);
  const html = await shopRes.text();
  assert.match(html, /Northwind Kits/);

  const hero = await fetch(`${base}/demo/hero.png`);
  assert.equal(hero.status, 200);
  assert.equal(hero.headers.get("content-type"), "image/png");

  const known = await fetch(`${base}/.well-known/stampede-abc123.txt`);
  assert.equal(known.status, 200);
  assert.equal(await known.text(), "abc123");
} finally {
  await new Promise((resolve, reject) => server.close((err) => (err ? reject(err) : resolve())));
}

console.log("smoke ok: shop, well-known, healthz");
