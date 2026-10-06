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

async function handler(req, res) {
  try {
    if ((req.method || "GET") !== "GET") {
      res.statusCode = 405;
      res.setHeader("Allow", "GET");
      res.end("GET only");
      return;
    }
    const host = first(req.headers["x-forwarded-host"]) || first(req.headers.host) || "localhost";
    const proto = first(req.headers["x-forwarded-proto"]) || "https";
    const incoming = new URL(req.url || "/", `${proto}://${host}`);
    const path = demoPath(incoming);
    const result = demoResult(new URL(path, `${proto}://${host}`));
    await sleep(result.delayMs);
    res.statusCode = result.status;
    res.setHeader("Content-Type", result.contentType);
    res.setHeader("Cache-Control", path === "/demo/hero.png" ? "public, max-age=3600" : "no-store");
    res.setHeader("X-Stampede-Demo", "northwind");
    res.end(result.body);
  } catch (err) {
    console.error(err);
    res.statusCode = 500;
    res.end("demo shop failed");
  }
}
function demoPath(incoming) {
  const rest = incoming.searchParams.get("path");
  if (rest) return `/demo/${rest.replace(/^\/+/, "")}`;
  if (incoming.pathname === "/demo" || incoming.pathname.startsWith("/demo/")) return incoming.pathname;
  return "/demo";
}
function first(value) {
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}
export {
  handler as default
};
