import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Challenge, type Config, type Preset, type Run, type Sample } from "./api";
import { Chart } from "./Chart";

export default function Home() {
  const [config, setConfig] = useState<Config | null>(null);
  const [presets, setPresets] = useState<Preset[]>([]);
  const [url, setUrl] = useState("");
  const [challenge, setChallenge] = useState<Challenge | null>(null);
  const [verified, setVerified] = useState(false);
  const [method, setMethod] = useState<"file" | "dns">("file");
  const [optIn, setOptIn] = useState(false);
  const [run, setRun] = useState<Run | null>(null);
  const [samples, setSamples] = useState<Sample[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const stageRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    api.config().then(setConfig).catch((err: Error) => setError(err.message));
    api.presets().then((r) => setPresets(r.presets)).catch((err: Error) => setError(err.message));
  }, []);

  useEffect(() => {
    if (!run || run.status !== "running") return;
    const es = new EventSource(`/v1/runs/${run.id}/events`);
    es.addEventListener("sample", (ev) => {
      const sample = JSON.parse((ev as MessageEvent).data) as Sample;
      setSamples((prev) => {
        const next = prev.filter((s) => s.sec !== sample.sec);
        next.push(sample);
        next.sort((a, b) => a.sec - b.sec);
        return next;
      });
    });
    const pull = () => {
      api
        .run(run.id)
        .then((next) => {
          setRun(next);
          if (next.samples && next.samples.length) setSamples(next.samples);
        })
        .catch((err: Error) => setError(err.message));
    };
    es.addEventListener("done", () => {
      es.close();
      pull();
    });
    es.onerror = () => {
      es.close();
      pull();
    };
    return () => es.close();
  }, [run?.id, run?.status]);

  useEffect(() => {
    if (challenge || run) {
      stageRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [challenge, verified, run?.status]);

  async function submitURL(raw: string) {
    setError("");
    setBusy(true);
    setVerified(false);
    setRun(null);
    setSamples([]);
    try {
      const ch = await api.challenge(raw);
      setChallenge(ch);
      setUrl(ch.url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not start verification");
    } finally {
      setBusy(false);
    }
  }

  async function verify() {
    if (!challenge) return;
    setError("");
    setBusy(true);
    try {
      await api.verify(challenge.id, method);
      setVerified(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Verification failed");
    } finally {
      setBusy(false);
    }
  }

  async function start(presetId: string) {
    if (!challenge) return;
    setError("");
    setBusy(true);
    setSamples([]);
    try {
      const started = await api.start(challenge.id, presetId, optIn);
      setRun(started);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not start the run");
    } finally {
      setBusy(false);
    }
  }

  const latest = samples[samples.length - 1];
  const demoHost = config?.demo_url ? safeHost(config.demo_url) : "";
  const onDemo = challenge && demoHost && challenge.host === demoHost;

  return (
    <div className="home">
      <p className="eyebrow">Launch-day load, with the safety on</p>
      <h1>Will your app survive the front page?</h1>
      <p className="lede">
        Find out in 60 seconds. Paste a URL you own. Stampede ramps a Product Hunt-shaped crowd
        and shows you the second it breaks.
      </p>
      <ul className="pills">
        <li>GET only</li>
        <li>{config ? `${config.max_rps} req/s cap` : "Rate cap"}</li>
        <li>3 minute max</li>
        <li>Ownership required</li>
      </ul>

      {config?.kill && (
        <p className="banner bad" role="status">
          Stampede is paused. New runs are refused until the kill switch is lifted.
        </p>
      )}
      {error && (
        <p className="banner bad" role="alert">
          {error}
        </p>
      )}

      <form
        className="url-form"
        onSubmit={(ev) => {
          ev.preventDefault();
          void submitURL(url);
        }}
      >
        <label htmlFor="url">Site URL</label>
        <div className="url-row">
          <span aria-hidden="true">stampede&gt;</span>
          <input
            id="url"
            name="url"
            inputMode="url"
            autoComplete="url"
            placeholder="https://yourapp.com"
            value={url}
            onChange={(ev) => setUrl(ev.target.value)}
            required
          />
          <button type="submit" disabled={busy || config?.kill}>
            Check
          </button>
        </div>
        {config?.demo_url && (
          <button
            type="button"
            className="ghost"
            onClick={() => {
              setUrl(config.demo_url);
              void submitURL(config.demo_url);
            }}
            disabled={busy || config.kill}
          >
            Use the demo shop
          </button>
        )}
      </form>

      <section ref={stageRef} className="stage">
        {challenge && !verified && (
          <div className="card">
            <h2>Prove you own {challenge.host}</h2>
            <p>
              {onDemo
                ? "This demo shop is ours. It will answer the token file. On a real site, you place the file or the DNS record yourself."
                : "Put the token on the host, then verify. Nothing is sent until this matches."}
            </p>
            <div className="token">
              <code>{challenge.token}</code>
              <button type="button" className="ghost" onClick={() => copy(challenge.token)}>
                Copy
              </button>
            </div>
            <div className="methods">
              <label className={method === "file" ? "on" : ""}>
                <input
                  type="radio"
                  name="method"
                  checked={method === "file"}
                  onChange={() => setMethod("file")}
                />
                <strong>Token file</strong>
                <span>
                  Serve <code>{challenge.file_path}</code> with the token as the body.
                </span>
              </label>
              <label className={method === "dns" ? "on" : ""}>
                <input
                  type="radio"
                  name="method"
                  checked={method === "dns"}
                  onChange={() => setMethod("dns")}
                />
                <strong>DNS TXT</strong>
                <span>
                  <code>{challenge.dns_name}</code> = <code>{challenge.dns_value}</code>
                </span>
              </label>
            </div>
            <button type="button" onClick={() => void verify()} disabled={busy}>
              {busy ? "Checking…" : "Verify ownership"}
            </button>
          </div>
        )}

        {verified && !run && (
          <div className="card">
            <h2>Pick the morning you are afraid of</h2>
            <p>Both curves ramp workers from 10 to 50. The rate stays under the safety cap.</p>
            <div className="presets">
              {presets.map((p) => (
                <article key={p.id} className="ticket">
                  <h3>{p.name}</h3>
                  <p>{p.blurb}</p>
                  <p className="meta">
                    {p.duration_seconds}s · peak {Math.round(p.peak_fraction * 100)}% of cap ·{" "}
                    {p.start_workers}→{p.end_workers} workers
                  </p>
                  <details>
                    <summary>Assumptions</summary>
                    <ul>
                      {p.assumptions.map((a) => (
                        <li key={a}>{a}</li>
                      ))}
                    </ul>
                  </details>
                  <button type="button" disabled={busy || config?.kill} onClick={() => void start(p.id)}>
                    Run this curve
                  </button>
                </article>
              ))}
            </div>
            <label className="optin">
              <input type="checkbox" checked={optIn} onChange={(ev) => setOptIn(ev.target.checked)} />
              Re-test this site nightly for 30 days
            </label>
          </div>
        )}

        {run && (
          <div className="card live">
            <div className="live-head">
              <h2>{run.status === "running" ? "Ramping" : run.report?.headline || "Run finished"}</h2>
              <span className={`status ${run.status}`}>{run.status}</span>
            </div>
            <div className="meters" aria-live="polite">
              <Meter label="Workers" value={latest ? String(latest.workers) : "—"} />
              <Meter label="Req/s" value={latest ? latest.rps.toFixed(0) : "—"} />
              <Meter label="p50" value={latest ? `${Math.round(latest.p50_ms)}ms` : "—"} />
              <Meter label="p95" value={latest ? `${Math.round(latest.p95_ms)}ms` : "—"} />
              <Meter
                label="Errors"
                value={latest ? `${(latest.error_rate * 100).toFixed(0)}%` : "—"}
                warn={!!latest && latest.error_rate > 0.05}
              />
            </div>
            <Chart
              samples={samples.length ? samples : run.samples || []}
              maxRps={run.max_rps}
              breakingRps={run.breaking_rps}
              hasBreaking={run.has_breaking}
            />
            <div className="legend">
              <span className="swatch rps" /> req/s
              <span className="swatch p95" /> p95
              <span className="swatch workers" /> workers
              {run.has_breaking && <span className="swatch break" />}
              {run.has_breaking && ` broke ~${Math.round(run.breaking_rps)} req/s`}
            </div>
            <div
              className="progress"
              aria-hidden="true"
              style={{
                transform: `scaleX(${Math.min(1, ((latest?.sec ?? 0) + 1) / Math.max(run.duration_seconds, 1))})`,
              }}
            />
            {run.has_breaking && run.status !== "running" && (
              <p className="banner warn">Breaking point around {Math.round(run.breaking_rps)} req/s.</p>
            )}
            {run.report && <ReportView run={run} />}
          </div>
        )}
      </section>
    </div>
  );
}

export function ReportView({ run }: { run: Run }) {
  const report = run.report;
  if (!report) return null;
  const origin = window.location.origin;
  const badge = `${origin}${run.badge_path}`;
  const page = `${origin}${run.result_path}`;
  const embed = `[![Stampede](${badge})](${page})`;
  return (
    <div className="report">
      <p className="summary">{report.summary}</p>
      {report.notice && <p className="note">{report.notice}</p>}
      <ul className="fixes">
        {report.fixes.map((fix) => (
          <li key={fix.title}>
            <strong>{fix.title}</strong>
            <span>{fix.detail}</span>
          </li>
        ))}
      </ul>
      <p className="cost">
        A launch day at this shape is about <strong>${report.cost.usd.toFixed(2)}</strong> on Cloud
        Run ({report.cost.instances} instance{report.cost.instances === 1 ? "" : "s"},{" "}
        {report.cost.requests.toLocaleString()} requests). {report.cost.note}
      </p>
      <div className="share">
        <img src={run.badge_path} alt={report.verdict === "launch_ready" ? "Launch-ready badge" : "Needs work badge"} />
        <div>
          <p>
            Share the result. <Link to={run.result_path}>Open the result page</Link>
          </p>
          <code>{embed}</code>
          <button type="button" className="ghost" onClick={() => copy(embed)}>
            Copy embed
          </button>
          <p className="note">Report model: {report.model}</p>
        </div>
      </div>
    </div>
  );
}

function Meter({ label, value, warn }: { label: string; value: string; warn?: boolean }) {
  return (
    <div className={warn ? "meter warn" : "meter"}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function copy(text: string) {
  void navigator.clipboard?.writeText(text);
}

function safeHost(raw: string) {
  try {
    return new URL(raw).hostname;
  } catch {
    return "";
  }
}
