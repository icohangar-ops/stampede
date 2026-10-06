import { createServer } from "node:http";
import { sleep } from "./lib/demo";
import { toRequest, writeResponse } from "./lib/node-adapter";
import { createApp, renderDemo } from "./lib/server";

const app = createApp({ trustProxy: true });
const port = Number(process.env.PORT || 8787);

createServer(async (req, res) => {
  try {
    const url = new URL(req.url || "/", "http://localhost");
    if (url.pathname === "/demo" || url.pathname.startsWith("/demo/")) {
      if ((req.method || "GET") !== "GET") {
        res.statusCode = 405;
        res.end("GET only");
        return;
      }
      const result = renderDemo(new URL(url.pathname, `http://${req.headers.host || "localhost"}`));
      await sleep(result.delayMs);
      res.statusCode = result.status;
      res.setHeader("Content-Type", result.contentType);
      res.setHeader("Cache-Control", "no-store");
      res.end(result.body);
      return;
    }
    if (url.pathname.startsWith("/.well-known/stampede-")) {
      const result = renderDemo(new URL(url.pathname, `http://${req.headers.host || "localhost"}`));
      res.statusCode = result.status;
      res.setHeader("Content-Type", result.contentType);
      res.end(result.body);
      return;
    }
    const request = await toRequest(req, "http");
    await writeResponse(res, await app.handle(request));
  } catch (err) {
    console.error(err);
    if (!res.headersSent) res.statusCode = 500;
    res.end(JSON.stringify({ error: "internal error" }));
  }
}).listen(port, () => {
  console.log(`Stampede API on http://127.0.0.1:${port}`);
});
