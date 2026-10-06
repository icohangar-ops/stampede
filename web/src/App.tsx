import { Link, Route, Routes } from "react-router-dom";
import Home from "./Home";
import Result from "./Result";
import Terms from "./Terms";

export default function App() {
  return (
    <>
      <a className="skip" href="#main">
        Skip to content
      </a>
      <header className="top">
        <Link to="/" className="mark">
          STAMPEDE
        </Link>
        <span className="tag">Cloud Run · Product Hunt</span>
      </header>
      <main id="main">
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/r/:id" element={<Result />} />
          <Route path="/terms" element={<Terms />} />
        </Routes>
      </main>
      <footer className="foot">
        <p>
          GET requests only. You must prove you own the host. Hard caps on rate, duration, and
          concurrency. We can stop a run with the kill switch.
        </p>
        <Link to="/terms">Terms</Link>
      </footer>
    </>
  );
}
