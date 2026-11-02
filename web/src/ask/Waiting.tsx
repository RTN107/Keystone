import { useEffect, useState } from "react";
import type { View } from "./state";

type W = Extract<View, { phase: "waiting" }>;

// Shown while the embeddings service is rate limited. This is correct behaviour on a
// plan with a low allowance, so it reads as a queue the question is standing in, with
// the whole wait drawn to scale: the longest Keystone will wait is (attempts - 1) pauses.
// Nothing here spins. The bar is the time actually waited, and the count is the time
// until the next try.
export function Waiting({ v }: { v: W }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(id);
  }, []);
  const pauses = Math.max(1, v.maxAttempts - 1);
  const total = pauses * v.retryIn; // seconds
  const sinceEvent = Math.max(0, (now - v.startedAt) / 1000);
  const waited = Math.min(total, (v.attempt - 1) * v.retryIn + sinceEvent);
  const next = Math.max(0, Math.ceil(v.retryIn - sinceEvent));
  const pct = (waited / total) * 100;
  return (
    <section className="waiting" aria-label="Waiting its turn">
      {/* The message is announced when it changes. The ticking numbers below are not. */}
      <p role="status" className="waiting-message">
        {v.message}
      </p>
      <div className="window" aria-hidden="true">
        <div className="window-track">
          <div className="window-fill" style={{ width: `${pct}%` }} />
          {Array.from({ length: pauses - 1 }, (_, i) => (
            <span key={i} className="window-tick" style={{ left: `${((i + 1) / pauses) * 100}%` }} />
          ))}
        </div>
      </div>
      <p className="waiting-next">
        <span>{next > 0 ? <>Trying again in <b className="tnum">{next}</b> s</> : "Trying again now"}</span>
        <span className="waiting-count">
          Attempt {Math.min(v.attempt + 1, v.maxAttempts)} of {v.maxAttempts}
        </span>
      </p>
    </section>
  );
}
