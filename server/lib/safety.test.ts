import assert from "node:assert/strict";
import { createServer } from "node:http";
import { once } from "node:events";
import test from "node:test";
import { assertGet, defaultPolicy, normalize, safeGet } from "./safety";

test("normalize blocks private, metadata, userinfo, odd ports, and obfuscated hosts", () => {
  const policy = defaultPolicy();
  for (const raw of [
    "http://127.0.0.1/",
    "http://10.1.2.3/",
    "http://192.168.1.9/admin",
    "http://169.254.169.254/latest",
    "http://169.254.169.253/",
    "http://100.64.1.1/",
    "http://192.0.2.10/",
    "http://metadata.google.internal/",
    "http://user:pass@example.com/",
    "http://example.com:8080/",
    "http://2130706433/",
    "file:///etc/passwd",
  ]) {
    assert.throws(() => normalize(raw, policy));
  }
  const allow = defaultPolicy({ allowHosts: new Set(["metadata.google.internal", "127.0.0.1"]) });
  assert.throws(() => normalize("http://metadata.google.internal/", allow));
  assert.throws(() => normalize("http://169.254.169.254/", allow));
  assert.equal(normalize("http://127.0.0.1:8090/demo", allow).port, 8090);
  assert.equal(normalize("https://Example.com/path", policy).host, "example.com");
});

test("safeGet refuses a redirect onto a metadata address and only sends GET", async () => {
  const seen: string[] = [];
  const server = createServer((req, res) => {
    seen.push(`${req.method} ${req.url}`);
    assert.equal(req.method, "GET");
    if (req.url === "/go") {
      res.writeHead(302, { Location: "http://169.254.169.254/latest/meta-data" });
      res.end();
      return;
    }
    res.writeHead(200, { "Content-Type": "text/plain" });
    res.end("ok");
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("no port");
  const policy = defaultPolicy({ allowHosts: new Set(["127.0.0.1"]) });
  await assert.rejects(() => safeGet(`http://127.0.0.1:${address.port}/go`, policy));
  const ok = await safeGet(`http://127.0.0.1:${address.port}/ok`, policy);
  assert.equal(ok.status, 200);
  assert.equal(ok.body.toString(), "ok");
  assert.deepEqual(seen, ["GET /go", "GET /ok"]);
  assert.throws(() => assertGet("POST"));
  server.close();
});
