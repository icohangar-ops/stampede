// Gate in front of every outbound connection.
// Caps, URL shape, and address class are enforced here so a caller
// cannot turn Stampede into a flood or an SSRF client.

import { lookup } from "node:dns/promises";
import http from "node:http";
import https from "node:https";
import { BlockList, isIP } from "node:net";
import { MAX_RPS, MAX_WORKERS, SAFETY_MAX_SECONDS, USER_AGENT } from "./limits";

export class PolicyError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "PolicyError";
  }
}

export type Resolver = (host: string) => Promise<string[]>;

export type Policy = {
  allowHosts: Set<string>;
  maxRps: number;
  maxDurationSeconds: number;
  maxWorkers: number;
  maxRedirects: number;
  resolver?: Resolver;
};

export type Target = {
  url: URL;
  host: string;
  port: number;
};

export function defaultPolicy(over: Partial<Policy> = {}): Policy {
  return {
    allowHosts: over.allowHosts ?? new Set<string>(),
    maxRps: over.maxRps ?? MAX_RPS,
    maxDurationSeconds: over.maxDurationSeconds ?? SAFETY_MAX_SECONDS,
    maxWorkers: over.maxWorkers ?? MAX_WORKERS,
    maxRedirects: over.maxRedirects ?? 2,
    resolver: over.resolver,
  };
}

const blocked = new BlockList();
for (const [addr, prefix, type] of [
  ["0.0.0.0", 8, "ipv4"],
  ["10.0.0.0", 8, "ipv4"],
  ["100.64.0.0", 10, "ipv4"],
  ["127.0.0.0", 8, "ipv4"],
  ["169.254.0.0", 16, "ipv4"],
  ["172.16.0.0", 12, "ipv4"],
  ["192.0.0.0", 24, "ipv4"],
  ["192.0.2.0", 24, "ipv4"],
  ["192.168.0.0", 16, "ipv4"],
  ["198.18.0.0", 15, "ipv4"],
  ["198.51.100.0", 24, "ipv4"],
  ["203.0.113.0", 24, "ipv4"],
  ["224.0.0.0", 4, "ipv4"],
  ["240.0.0.0", 4, "ipv4"],
  ["::1", 128, "ipv6"],
  ["fc00::", 7, "ipv6"],
  ["fe80::", 10, "ipv6"],
  ["2001:db8::", 32, "ipv6"],
] as const) {
  blocked.addSubnet(addr, prefix, type);
}

export function normalize(raw: string, policy: Policy): Target {
  const trimmed = raw.trim();
  if (!trimmed || trimmed.length > 2048 || /[\\\n\r]/.test(trimmed)) {
    throw new PolicyError("host is not allowed");
  }
  let url: URL;
  try {
    url = new URL(trimmed);
  } catch {
    throw new PolicyError("host is not allowed");
  }
  if (url.username || url.password) throw new PolicyError("URLs with userinfo are blocked");
  const scheme = url.protocol.replace(":", "").toLowerCase();
  if (scheme !== "http" && scheme !== "https") throw new PolicyError("only http and https URLs are allowed");
  const host = normalizeHost(url.hostname);
  if (!host) throw new PolicyError("host is not allowed");
  if (isMetadataHost(host)) throw new PolicyError("cloud metadata address blocked");
  if (isObfuscatedHost(host)) throw new PolicyError("obfuscated IP host is blocked");
  let port = scheme === "https" ? 443 : 80;
  if (url.port) {
    const n = Number(url.port);
    if (!Number.isInteger(n) || n < 1 || n > 65535) throw new PolicyError("port is not allowed");
    port = n;
  }
  const allow = policy.allowHosts.has(host);
  if (isIP(host)) assertIp(host, allow);
  else if (!host.includes(".") && !allow) throw new PolicyError("host is not allowed");
  if (!allowedPort(port, allow)) throw new PolicyError("port is not allowed");
  url.hash = "";
  url.protocol = `${scheme}:`;
  url.hostname = host;
  url.port = port === 80 && scheme === "http" || port === 443 && scheme === "https" ? "" : String(port);
  return { url, host, port };
}

export function validatePlan(policy: Policy, rps: number, seconds: number, workers: number): void {
  if (rps <= 0 || seconds < 1 || workers <= 0) throw new PolicyError("requested load exceeds the safety cap");
  if (rps > policy.maxRps || seconds > policy.maxDurationSeconds || workers > policy.maxWorkers) {
    throw new PolicyError("requested load exceeds the safety cap");
  }
  if (seconds > SAFETY_MAX_SECONDS) throw new PolicyError("requested load exceeds the safety cap");
}

export function assertGet(method: string): void {
  if (method !== "GET") throw new PolicyError("only GET is allowed");
}

export type GetResult = {
  status: number;
  body: Buffer;
  ms: number;
  contentType: string;
  finalUrl: string;
};

export async function safeGet(
  raw: string,
  policy: Policy,
  opts: { maxBytes?: number; timeoutMs?: number } = {},
): Promise<GetResult> {
  const maxBytes = opts.maxBytes ?? 1 << 20;
  const timeoutMs = opts.timeoutMs ?? 8000;
  let current = normalize(raw, policy);
  for (let hop = 0; hop <= policy.maxRedirects; hop++) {
    const ip = await resolveHost(current.host, policy);
    const result = await requestOnce(current, ip, maxBytes, timeoutMs);
    const location = result.location;
    if (result.status >= 300 && result.status < 400 && location) {
      if (hop === policy.maxRedirects) throw new PolicyError("redirect blocked");
      const next = new URL(location, current.url);
      if (normalizeHost(next.hostname) !== current.host) throw new PolicyError("redirect blocked");
      current = normalize(next.toString(), policy);
      continue;
    }
    return {
      status: result.status,
      body: result.body,
      ms: result.ms,
      contentType: result.contentType,
      finalUrl: current.url.toString(),
    };
  }
  throw new PolicyError("redirect blocked");
}

async function resolveHost(host: string, policy: Policy): Promise<string> {
  const allow = policy.allowHosts.has(normalizeHost(host));
  if (isMetadataHost(host)) throw new PolicyError("cloud metadata address blocked");
  if (isIP(host)) {
    assertIp(host, allow);
    return canonical(host).addr;
  }
  const addrs = policy.resolver
    ? await policy.resolver(host)
    : (await lookup(host, { all: true, verbatim: true })).map((a) => a.address);
  if (!addrs.length) throw new PolicyError("host is not allowed");
  let picked = "";
  for (const addr of addrs) {
    assertIp(addr, allow);
    const kind = isIP(addr);
    if (kind === 4) picked = canonical(addr).addr;
    else if (!picked) picked = canonical(addr).addr;
  }
  if (!picked) throw new PolicyError("private or internal address blocked");
  return picked;
}

type Raw = { status: number; body: Buffer; ms: number; contentType: string; location?: string };

function requestOnce(target: Target, ip: string, maxBytes: number, timeoutMs: number): Promise<Raw> {
  assertGet("GET");
  const started = Date.now();
  const lib = target.url.protocol === "https:" ? https : http;
  const path = `${target.url.pathname}${target.url.search}` || "/";
  return new Promise((resolve, reject) => {
    const req = lib.request(
      {
        host: ip,
        port: target.port,
        path,
        method: "GET",
        headers: {
          Host: target.url.host,
          "User-Agent": USER_AGENT,
          Accept: "text/html,application/xhtml+xml,text/plain,*/*;q=0.8",
          Connection: "close",
        },
        servername: target.host,
        timeout: timeoutMs,
        agent: false,
      },
      (res) => {
        const chunks: Buffer[] = [];
        let size = 0;
        res.on("data", (chunk: Buffer) => {
          size += chunk.length;
          if (size <= maxBytes) chunks.push(chunk);
          else res.destroy();
        });
        res.on("end", () => {
          resolve({
            status: res.statusCode || 0,
            body: Buffer.concat(chunks),
            ms: Date.now() - started,
            contentType: String(res.headers["content-type"] || ""),
            location: headerOne(res.headers.location),
          });
        });
        res.on("error", reject);
      },
    );
    req.on("timeout", () => req.destroy(new Error("timeout")));
    req.on("error", reject);
    req.end();
  });
}

function headerOne(value: string | string[] | undefined): string | undefined {
  if (Array.isArray(value)) return value[0];
  return value;
}

function assertIp(addr: string, allow: boolean): void {
  const { addr: ip, type } = canonical(addr);
  if (isMetadataIp(ip)) throw new PolicyError("cloud metadata address blocked");
  if (!allow && blocked.check(ip, type)) throw new PolicyError("private or internal address blocked");
}

function canonical(addr: string): { addr: string; type: "ipv4" | "ipv6" } {
  const lower = addr.toLowerCase();
  if (lower.startsWith("::ffff:")) {
    const v4 = lower.slice(7);
    if (isIP(v4) === 4) return { addr: v4, type: "ipv4" };
  }
  return { addr: lower, type: isIP(lower) === 6 ? "ipv6" : "ipv4" };
}

function isMetadataIp(ip: string): boolean {
  const parts = ip.split(".").map((n) => Number(n));
  if (parts.length !== 4 || parts.some((n) => !Number.isInteger(n))) return false;
  return parts[0] === 169 && parts[1] === 254 && parts[2] === 169 && (parts[3] === 254 || parts[3] === 253);
}

function isMetadataHost(host: string): boolean {
  switch (normalizeHost(host)) {
    case "metadata.google.internal":
    case "metadata.goog":
    case "metadata":
      return true;
    default:
      return false;
  }
}

function isObfuscatedHost(host: string): boolean {
  return host.length > 0 && /^[0-9]+$/.test(host);
}

function allowedPort(port: number, allowlisted: boolean): boolean {
  if (port < 1 || port > 65535) return false;
  if (allowlisted) return true;
  return port === 80 || port === 443;
}

export function normalizeHost(host: string): string {
  return host.toLowerCase().trim().replace(/\.$/, "");
}
