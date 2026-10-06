// Safety ceilings. Client input cannot raise them.
// The Vercel deployment also clamps curve length so a run finishes inside maxDuration.

export const MAX_RPS = 40;
export const MAX_WORKERS = 50;
export const SAFETY_MAX_SECONDS = 180;
export const DEFAULT_PLATFORM_MAX_SECONDS = 40;
export const DEFAULT_DURATION_SECONDS = 30;
export const DOMAIN_QUOTA = 3;
export const IP_QUOTA = 5;
export const CHALLENGE_LIMIT_PER_HOUR = 30;
export const USER_AGENT = "StampedeBot/1.0 (launch check; ownership-verified GET only)";

function intEnv(name: string, fallback: number, min: number, max: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const n = Number(raw);
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, Math.floor(n)));
}

export function platformMaxSeconds(): number {
  return intEnv("STAMPEDE_PLATFORM_MAX_SECONDS", DEFAULT_PLATFORM_MAX_SECONDS, 10, SAFETY_MAX_SECONDS);
}

export function defaultDurationSeconds(): number {
  return intEnv("STAMPEDE_DURATION_SECONDS", DEFAULT_DURATION_SECONDS, 10, platformMaxSeconds());
}

export function maxRps(): number {
  const raw = process.env.STAMPEDE_MAX_RPS;
  if (!raw) return MAX_RPS;
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0) return MAX_RPS;
  return Math.min(MAX_RPS, n);
}

export function allowHosts(): Set<string> {
  const out = new Set<string>();
  for (const part of (process.env.STAMPEDE_ALLOW_HOSTS || "").split(",")) {
    const host = part.trim().toLowerCase().replace(/\.$/, "");
    if (host) out.add(host);
  }
  return out;
}
