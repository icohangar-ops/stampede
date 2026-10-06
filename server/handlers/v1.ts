import type { IncomingMessage, ServerResponse } from "node:http";
import { toRequest, writeResponse } from "../lib/node-adapter";
import { createApp } from "../lib/server";

const app = createApp();

export const config = {
  maxDuration: 90,
};

// Single file, not a catch-all. A `[...slug]` route under api/ is skipped when the
// SPA rewrite is present, so /v1/* and /api/v1/* are rewritten onto this function
// with the remainder in ?path=.
export default async function handler(req: IncomingMessage, res: ServerResponse): Promise<void> {
  try {
    req.url = v1Url(req.url || "/");
    const request = await toRequest(req, "https");
    const response = await app.handle(request);
    await writeResponse(res, response);
  } catch (err) {
    console.error(err);
    if (!res.headersSent) {
      res.statusCode = 500;
      res.setHeader("Content-Type", "application/json");
    }
    res.end(JSON.stringify({ error: "internal error" }));
  }
}

export function v1Url(raw: string): string {
  const url = new URL(raw || "/", "https://localhost");
  if (url.pathname !== "/api/v1" && url.pathname !== "/v1") return raw || "/";
  if (!url.searchParams.has("path")) return raw || "/";
  const routed = url.searchParams.get("path") || "";
  url.searchParams.delete("path");
  const suffix = routed.replace(/^\/+/, "");
  url.pathname = suffix ? `/v1/${suffix}` : "/v1";
  return `${url.pathname}${url.search}`;
}
