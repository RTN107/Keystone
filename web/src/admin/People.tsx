import { useState, type FormEvent } from "react";
import { type TeamWithMembers, createEmployee } from "./adminApi";

const COUNT_WORDS = ["No", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"];

/** "Three teams" for small counts, the number itself above ten. */
function teamCount(n: number): string {
  return `${n <= 10 ? COUNT_WORDS[n] : n} ${n === 1 ? "team" : "teams"}`;
}

export function People({ teams, readOnly, onChanged }: { teams: TeamWithMembers[]; readOnly: boolean; onChanged: () => void }) {
  const [name, setName] = useState("");
  const [teamId, setTeamId] = useState(0);
  const [password, setPassword] = useState("");
  const [number, setNumber] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [made, setMade] = useState<string | null>(null);
  const exampleTeam = teams.find((t) => t.id === teamId) ?? teams[0];
  const numberExample = `${exampleTeam ? exampleTeam.number_prefix : "ABC"}-002`;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy || readOnly) return;
    setBusy(true);
    setError(null);
    setMade(null);
    try {
      const emp = await createEmployee({ name, team_id: teamId, password, employee_number: number.trim() || undefined });
      setMade(`${emp.name} can sign in as ${emp.employee_number}.`);
      setName("");
      setPassword("");
      setNumber("");
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : "The account was not created. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <header className="page-head">
        <h1 id="view-title">People</h1>
        {teams.length === 0 ? (
          <p>No teams exist yet. Set KEYSTONE_TEAMS on the API and restart it, then they appear here.</p>
        ) : (
          <p>{teamCount(teams.length)}. An employee reads the documents their team can read, and nothing else.</p>
        )}
      </header>
      <div className="people">
        {teams.map((t) => (
          <section key={t.id} className="block" aria-labelledby={`team-${t.id}`}>
            <h2 id={`team-${t.id}`} className="block-title">{t.name}</h2>
            {t.members.length === 0 ? (
              <p className="quiet">No one yet.</p>
            ) : (
              <ul className="members">
                {t.members.map((m) => (
                  <li key={m.id}>
                    <span className="num-tag">{m.employee_number}</span> {m.name}
                  </li>
                ))}
              </ul>
            )}
          </section>
        ))}
      </div>
      <form className="block add" onSubmit={submit} aria-labelledby="add-title">
        <h2 id="add-title" className="block-title">Add an employee</h2>
        <fieldset className="plain" disabled={readOnly || busy}>
          <label className="field">
            <span>Name</span>
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} autoComplete="off" required />
          </label>
          <label className="field">
            <span>Team</span>
            <select className="input" value={teamId} onChange={(e) => setTeamId(Number(e.target.value))} required>
              <option value={0} disabled>Choose a team</option>
              {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
            </select>
          </label>
          <label className="field">
            <span>Employee number <em className="optional">optional</em></span>
            <input className="input" value={number} onChange={(e) => setNumber(e.target.value)} autoCapitalize="characters" spellCheck={false} autoComplete="off" aria-describedby="number-hint" />
            <small id="number-hint" className="hint">Left empty, the next number for the team is used, such as {numberExample}.</small>
          </label>
          <label className="field">
            <span>Password</span>
            <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" minLength={10} required aria-describedby="pw-hint" />
            <small id="pw-hint" className="hint">At least 10 characters.</small>
          </label>
        </fieldset>
        <div className="actions">
          <button type="submit" className="btn" disabled={busy || readOnly || !name.trim() || !teamId || password.length < 10}>Add employee</button>
          {readOnly && <span className="quiet">This deployment is read-only, so accounts cannot be added.</span>}
        </div>
        <div aria-live="polite">
          {error && <p className="problem-line" role="alert">{error}</p>}
          {made && <p className="result"><strong>{made}</strong></p>}
        </div>
      </form>
    </>
  );
}
