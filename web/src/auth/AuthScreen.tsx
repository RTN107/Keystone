import { useState, type FormEvent } from "react";
import { api, ApiError, tokenStore, type Employee, type Health, type Session } from "../api";
import { Mark } from "../ui/Mark";
import "./auth.css";

// First-run setup and sign in share one quiet card. The server decides which one applies.
export function AuthScreen({ health, onSignedIn }: { health: Health; onSignedIn: (e: Employee) => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const setup = health.needs_setup;

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true);
    setError(null);
    try {
      const s = await api<Session>(setup ? "/api/setup" : "/api/auth/login", {
        method: "POST",
        token: null,
        body: setup
          ? { name: String(f.get("name")), password: String(f.get("password")) }
          : { employee_number: String(f.get("employee_number")), password: String(f.get("password")) },
      });
      tokenStore.set(s.token);
      onSignedIn(s.employee);
    } catch (err) {
      setError(err instanceof ApiError || err instanceof Error ? err.message : "Something went wrong.");
      setBusy(false);
    }
  }

  return (
    <main id="main" className="auth">
      <form className="auth-card" onSubmit={submit} aria-labelledby="auth-title">
        <div className="auth-mark">
          <Mark size={38} />
        </div>
        <h1 id="auth-title">{setup ? "Create the admin account" : "Sign in to Keystone"}</h1>
        {setup && <p className="auth-lede">This is the first run. The account you create here becomes the admin, ADM-001.</p>}
        {!setup && <p className="auth-lede">Use the employee number and password your admin gave you.</p>}

        {error && (
          <p className="auth-error" role="alert">
            {error}
          </p>
        )}
        {setup ? (
          <>
            <label className="field">
              <span>Your name</span>
              <input className="input" name="name" autoComplete="name" required />
            </label>
            <label className="field">
              <span>Password, at least 10 characters</span>
              <input className="input" name="password" type="password" autoComplete="new-password" minLength={10} required />
            </label>
          </>
        ) : (
          <>
            <label className="field">
              <span>Employee number</span>
              <input className="input" name="employee_number" autoComplete="username" autoCapitalize="characters" spellCheck={false} required />
            </label>
            <label className="field">
              <span>Password</span>
              <input className="input" name="password" type="password" autoComplete="current-password" required />
            </label>
          </>
        )}
        <button className="btn auth-submit" type="submit" disabled={busy}>
          {busy ? "One moment" : setup ? "Create admin account" : "Sign in"}
        </button>
      </form>
    </main>
  );
}
