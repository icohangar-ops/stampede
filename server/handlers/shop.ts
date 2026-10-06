import type { IncomingMessage, ServerResponse } from "node:http";
import { demoResult, sleep } from "../lib/demo";

// One function for /demo and /demo/*. Rewrites pass the rest of the path as ?path=.
export default async function handler(req: IncomingMessage, res: ServerResponse): Promise<void> {
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

function demoPath(incoming: URL): string {
  const rest = incoming.searchParams.get("path");
  if (rest) return `/demo/${rest.replace(/^\/+/, "")}`;
  if (incoming.pathname === "/demo" || incoming.pathname.startsWith("/demo/")) return incoming.pathname;
  return "/demo";
}

function first(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}
