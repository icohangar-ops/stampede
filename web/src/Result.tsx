import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, type Run } from "./api";
import { Chart } from "./Chart";
import { ReportView } from "./Home";

export default function Result() {
  const { id } = useParams();
  const [run, setRun] = useState<Run | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!id) return;
    let stop = false;
    const tick = () => {
      api
        .run(id)
        .then((next) => {
          if (stop) return;
          setRun(next);
          if (next.status === "running") setTimeout(tick, 1000);
        })
        .catch((err: Error) => {
          if (!stop) setError(err.message);
        });
    };
    tick();
    return () => {
      stop = true;
    };
  }, [id]);

  if (error) {
    return (
      <p className="banner bad" role="alert">
        {error}
      </p>
    );
  }
  if (!run) return <p className="lede">Loading the run…</p>;

  return (
    <article className="card result">
      <p className="eyebrow">{run.host}</p>
      <h1>{run.report?.headline || "Run in progress"}</h1>
      <div className="meters">
        <div className="meter">
          <span>Peak req/s</span>
          <strong>{run.peak_rps ? Math.round(run.peak_rps) : "—"}</strong>
        </div>
        <div className="meter">
          <span>Peak p95</span>
          <strong>{run.peak_p95_ms ? `${Math.round(run.peak_p95_ms)}ms` : "—"}</strong>
        </div>
        <div className="meter">
          <span>Errors</span>
          <strong>{run.error_rate ? `${(run.error_rate * 100).toFixed(1)}%` : "0%"}</strong>
        </div>
      </div>
      <Chart
        samples={run.samples || []}
        maxRps={run.max_rps}
        breakingRps={run.breaking_rps}
        hasBreaking={run.has_breaking}
      />
      {run.report && <ReportView run={run} />}
      <p>
        <Link to="/">Run another site</Link>
      </p>
    </article>
  );
}
