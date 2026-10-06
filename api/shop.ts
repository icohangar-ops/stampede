import type { IncomingMessage, ServerResponse } from "node:http";
import { sleep } from "./lib/demo";
import { renderDemo } from "./lib/server";

// One function for /demo and /demo/* so the route does not depend on an optional catch-all.
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
    const rest = incoming.searchParams.get("path") || "";
    const path = rest && rest !== "/" ? `/demo/${rest.replace(/^\/+/, "")}` : "/demo";
    const target = new URL(path, `${proto}://${host}`);
    const result = renderDemo(target);
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

function first(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}
