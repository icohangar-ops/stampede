import { Link } from "react-router-dom";

export default function Terms() {
  return (
    <article className="card terms">
      <p className="eyebrow">The rules</p>
      <h1>Terms</h1>
      <p>Stampede is a launch check for a site you control. It is not a flood tool.</p>
      <ul>
        <li>You may only test a site you own, or a site you are explicitly authorized to test.</li>
        <li>Ownership is verified with a token file or a DNS TXT record before any load is sent.</li>
        <li>Traffic is HTTP GET only. There is no request body and no other method.</li>
        <li>
          Runs are capped. The default ceiling is 40 requests per second, 50 parallel workers, and
          3 minutes. A domain and a client network each have a daily quota.
        </li>
        <li>
          Private, loopback, link-local, and cloud metadata addresses are blocked. The operator can
          allowlist the demo shop. Metadata stays blocked anyway.
        </li>
        <li>We can stop new runs and in-flight load with a kill switch, without notice.</li>
        <li>
          The readiness report and the Cloud Run cost figure are estimates from a short, capped
          test. They are not a guarantee and not an invoice.
        </li>
        <li>Opt-in nightly retests stop after 30 days, or sooner if you are over quota.</li>
      </ul>
      <p>
        <Link to="/">Back to the ramp</Link>
      </p>
    </article>
  );
}
