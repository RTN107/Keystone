import { useState } from "react";
import { type Outcome, type TeamWithMembers, listQueryLogs } from "./adminApi";
import { useLoad, when } from "./useLoad";

const OUTCOMES: { value: Outcome | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "answered", label: "Answered" },
  { value: "blocked", label: "Blocked" },
  { value: "no_match", label: "No match" },
];
const LABEL: Record<Outcome, string> = { answered: "Answered", blocked: "Blocked", no_match: "No match" };
const STEP = 50;

export function QueryLog({ teams }: { teams: TeamWithMembers[] }) {
  const [teamId, setTeamId] = useState(0);
  const [employee, setEmployee] = useState("");
  const [outcome, setOutcome] = useState<Outcome | "all">("all");
  const [limit, setLimit] = useState(STEP);
  const log = useLoad(() => listQueryLogs({ teamId, employee, limit }), [teamId, employee, limit]);
  const people = teams.filter((t) => !teamId || t.id === teamId).flatMap((t) => t.members.map((m) => ({ ...m, team: t.name })));
  const rows = (log.data ?? []).filter((q) => outcome === "all" || q.outcome === outcome);
  const more = (log.data?.length ?? 0) >= limit && limit < 200;

  return (
    <>
      <header className="page-head">
        <h1 id="view-title">Query log</h1>
        <p>Every question asked, and what Keystone did with it. Blocked means the closest document is one the team cannot read.</p>
      </header>
      <form className="filters" onSubmit={(e) => e.preventDefault()} aria-label="Filter the log">
        <label className="field">
          <span>Team</span>
          <select className="input" value={teamId} onChange={(e) => { setTeamId(Number(e.target.value)); setEmployee(""); setLimit(STEP); }}>
            <option value={0}>All teams</option>
            {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </select>
        </label>
        <label className="field">
          <span>Employee</span>
          <select className="input" value={employee} onChange={(e) => { setEmployee(e.target.value); setLimit(STEP); }}>
            <option value="">Everyone</option>
            {people.map((p) => <option key={p.id} value={p.employee_number}>{p.employee_number}, {p.name}</option>)}
          </select>
        </label>
        <fieldset className="plain seg">
          <legend>Outcome</legend>
          {OUTCOMES.map((o) => (
            <label key={o.value} className={"seg-item" + (outcome === o.value ? " is-on" : "")}>
              <input type="radio" name="outcome" checked={outcome === o.value} onChange={() => setOutcome(o.value)} />
              <span>{o.label}</span>
            </label>
          ))}
        </fieldset>
      </form>

      {log.error && <p className="problem-line" role="alert">{log.error}</p>}
      {log.loading && !log.data && <p className="quiet" role="status">Loading the log.</p>}
      {log.data && rows.length === 0 && <p className="quiet">No questions match these filters.</p>}
      {rows.length > 0 && (
        <table className="table queries">
          <thead>
            <tr>
              <th scope="col">When</th>
              <th scope="col">Who</th>
              <th scope="col">Question</th>
              <th scope="col">Outcome</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((q) => (
              <tr key={q.id}>
                <td data-label="When" className="nowrap">{when(q.at)}</td>
                <td data-label="Who">
                  <span className="doc-title">{q.employee_name}</span>
                  <span className="doc-file">{q.employee_number}, {q.team}</span>
                </td>
                <td data-label="Question" className="question-cell">
                  <span>{q.question}</span>
                  {q.answer && (
                    <details className="answer-detail">
                      <summary>{q.outcome === "answered" ? "Answer" : "Message shown"}</summary>
                      <p>{q.answer}</p>
                      {q.outcome === "answered" && <p className="doc-file">{q.cited_chunk_ids.length} {q.cited_chunk_ids.length === 1 ? "passage" : "passages"} cited</p>}
                    </details>
                  )}
                </td>
                <td data-label="Outcome"><span className={`outcome is-${q.outcome}`}>{LABEL[q.outcome]}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {more && (
        <p className="more">
          <button type="button" className="btn btn-quiet" onClick={() => setLimit((l) => Math.min(200, l + STEP))}>Show more</button>
        </p>
      )}
    </>
  );
}
