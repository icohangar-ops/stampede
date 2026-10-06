export type Config = {
  demo_url: string;
  max_rps: number;
  max_duration_seconds: number;
  max_workers: number;
  kill: boolean;
  model: string;
  domain_quota: number;
  ip_quota: number;
};

export type Preset = {
  id: string;
  name: string;
  blurb: string;
  assumptions: string[];
  duration_seconds: number;
  start_workers: number;
  end_workers: number;
  peak_fraction: number;
};

export type Challenge = {
  id: string;
  url: string;
  host: string;
  token: string;
  file_url: string;
  file_path: string;
  dns_name: string;
  dns_value: string;
  expires_at: string;
  verified: boolean;
  method: string;
};

export type Fix = { title: string; detail: string };
export type Cost = { usd: number; instances: number; requests: number; note: string };
export type Report = {
  verdict: string;
  headline: string;
  summary: string;
  fixes: Fix[];
  degrades_past_rps: number;
  has_breaking: boolean;
  cost: Cost;
  model: string;
  notice?: string;
};

export type Sample = {
  sec: number;
  workers: number;
  rps: number;
  p50_ms: number;
  p95_ms: number;
  error_rate: number;
  requests: number;
  errors: number;
};

export type Run = {
  id: string;
  url: string;
  host: string;
  preset: string;
  status: string;
  started_at: string;
  ended_at?: string;
  max_rps: number;
  duration_seconds: number;
  has_breaking: boolean;
  breaking_rps: number;
  peak_rps: number;
  peak_p95_ms: number;
  error_rate: number;
  verdict: string;
  report: Report | null;
  samples?: Sample[];
  error?: string;
  badge_path: string;
  result_path: string;
};

async function parse<T>(res: Response): Promise<T> {
  const body = (await res.json().catch(() => ({}))) as T & { error?: string };
  if (!res.ok) {
    throw new Error(body.error || `Request failed (${res.status})`);
  }
  return body;
}

export const api = {
  config: () => fetch("/v1/config").then((r) => parse<Config>(r)),
  presets: () => fetch("/v1/presets").then((r) => parse<{ presets: Preset[] }>(r)),
  challenge: (url: string) =>
    fetch("/v1/challenges", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    }).then((r) => parse<Challenge>(r)),
  verify: (id: string, method: "file" | "dns") =>
    fetch(`/v1/challenges/${id}/verify`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ method }),
    }).then((r) => parse<{ verified: boolean; method: string }>(r)),
  start: (challengeId: string, preset: string, optIn: boolean) =>
    fetch("/v1/runs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ challenge_id: challengeId, preset, opt_in: optIn }),
    }).then((r) => parse<Run>(r)),
  run: (id: string) => fetch(`/v1/runs/${id}`).then((r) => parse<Run>(r)),
};
