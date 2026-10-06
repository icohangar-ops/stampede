// Northwind Kits, the shop this deployment owns.
// It answers the ownership file and folds on purpose once the ramp gets serious.

const HERO_PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
  "base64",
);

const PAGE = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Northwind Kits</title></head>
<body>
  <h1>Northwind Kits</h1>
  <p>A small shop, about to meet the front page.</p>
  <img src="/demo/hero.png" alt="Kit on a bench" width="480" height="270">
  <p><a href="/demo/api/pricing">Pricing</a></p>
</body></html>`;

export type Gate = {
  hit: () => { delayMs: number; fail: boolean };
};

export function createGate(): Gate {
  const hits: number[] = [];
  return {
    hit() {
      const now = Date.now();
      const cut = now - 1000;
      while (hits.length && hits[0]! <= cut) hits.shift();
      hits.push(now);
      const n = hits.length;
      if (n >= 34) return { delayMs: 30, fail: true };
      if (n >= 24) return { delayMs: 1600, fail: false };
      if (n >= 14) return { delayMs: 220, fail: false };
      return { delayMs: 20, fail: false };
    },
  };
}

export const demoGate = createGate();

export type DemoResult = {
  status: number;
  body: Buffer;
  contentType: string;
  delayMs: number;
};

export function demoResult(url: URL, gate: Gate = demoGate): DemoResult {
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
      delayMs: 900,
    };
  }
  if (path === "/demo" || path === "/demo/") {
    const decision = gate.hit();
    if (decision.fail) {
      return {
        status: 503,
        body: Buffer.from("overloaded"),
        contentType: "text/plain; charset=utf-8",
        delayMs: decision.delayMs,
      };
    }
    return {
      status: 200,
      body: Buffer.from(PAGE),
      contentType: "text/html; charset=utf-8",
      delayMs: decision.delayMs,
    };
  }
  return { status: 404, body: Buffer.from("not found"), contentType: "text/plain; charset=utf-8", delayMs: 0 };
}

export function isDemoPath(url: URL, selfHost: string): boolean {
  return url.hostname.toLowerCase() === selfHost.toLowerCase() && (url.pathname === "/demo" || url.pathname === "/demo/");
}

export async function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  if (ms <= 0) return;
  await new Promise<void>((resolve) => {
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
