import type { IncomingMessage, ServerResponse } from "node:http";
import { toRequest, writeResponse } from "../lib/node-adapter";
import { createApp } from "../lib/server";

const app = createApp();

export const config = {
  maxDuration: 90,
};

export default async function handler(req: IncomingMessage, res: ServerResponse): Promise<void> {
  try {
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
