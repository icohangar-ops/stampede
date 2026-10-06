import type { IncomingMessage, ServerResponse } from "node:http";
import { renderDemo } from "../server/lib/server";

export default function handler(req: IncomingMessage, res: ServerResponse): void {
  const host = first(req.headers["x-forwarded-host"]) || first(req.headers.host) || "localhost";
  const proto = first(req.headers["x-forwarded-proto"]) || "https";
  const incoming = new URL(req.url || "/", `${proto}://${host}`);
  const token = incoming.searchParams.get("token") || "";
  const target = new URL(`/.well-known/stampede-${token}.txt`, `${proto}://${host}`);
  const result = renderDemo(target);
  res.statusCode = result.status;
  res.setHeader("Content-Type", result.contentType);
  res.setHeader("Cache-Control", "no-store");
  res.end(result.body);
}

function first(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}
