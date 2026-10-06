import type { IncomingMessage, ServerResponse } from "node:http";

export async function toRequest(req: IncomingMessage, fallbackProto = "http"): Promise<Request> {
  const host = header(req, "x-forwarded-host") || header(req, "host") || "localhost";
  const proto = header(req, "x-forwarded-proto") || fallbackProto;
  const url = new URL(req.url || "/", `${proto}://${host}`);
  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (typeof value === "string") headers.set(key, value);
    else if (Array.isArray(value)) value.forEach((item) => headers.append(key, item));
  }
  const method = req.method || "GET";
  if (method === "GET" || method === "HEAD") return new Request(url, { method, headers });
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(Buffer.from(chunk));
  return new Request(url, { method, headers, body: Buffer.concat(chunks) });
}

export async function writeResponse(res: ServerResponse, response: Response): Promise<void> {
  res.statusCode = response.status;
  response.headers.forEach((value, key) => {
    res.setHeader(key, value);
  });
  if (response.headers.get("content-type")?.includes("text/event-stream")) {
    res.flushHeaders?.();
  }
  if (!response.body) {
    res.end();
    return;
  }
  const reader = response.body.getReader();
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      res.write(Buffer.from(value));
    }
  } finally {
    res.end();
  }
}

function header(req: IncomingMessage, name: string): string {
  const value = req.headers[name];
  if (Array.isArray(value)) return value[0] || "";
  return value || "";
}
