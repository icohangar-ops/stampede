import type { IncomingMessage, ServerResponse } from "node:http";
import { demoResult } from "../lib/demo";

export default function handler(req: IncomingMessage, res: ServerResponse): void {
  const host = first(req.headers["x-forwarded-host"]) || first(req.headers.host) || "localhost";
  const proto = first(req.headers["x-forwarded-proto"]) || "https";
  const incoming = new URL(req.url || "/", `${proto}://${host}`);
  const token = ownershipToken(incoming);
  const result = demoResult(new URL(`/.well-known/stampede-${token}.txt`, `${proto}://${host}`));
  res.statusCode = result.status;
  res.setHeader("Content-Type", result.contentType);
  res.setHeader("Cache-Control", "no-store");
  res.end(result.body);
}

function ownershipToken(incoming: URL): string {
  const fromQuery = incoming.searchParams.get("token");
  if (fromQuery) return fromQuery;
  const match = incoming.pathname.match(/^\/\.well-known\/stampede-(.+)\.txt$/);
  return match?.[1] || "";
}

function first(value: string | string[] | undefined): string {
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}
