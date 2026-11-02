import { useEffect, useRef, useState, type FormEvent } from "react";
import { type AccessResult, type DeleteResult, type TeamWithMembers, deleteDocument, extendAccess, listDocuments, revokeAccess } from "./adminApi";
import { useLoad } from "./useLoad";

// The access screen: who can read a document, giving another team access, taking an extension back,
// and (at the bottom, behind a second step) deleting the document. It is the one admin screen that
// carries weight: it sits between a refusal and the answer that follows it.
// The words are the product here. The button names what will happen, the panel above it says who gains
// what and when, and the result says it once more in the past tense.
export function ExtendAccess({ docId, teams, readOnly }: { docId: number; teams: TeamWithMembers[]; readOnly: boolean }) {
  const docs = useLoad(() => listDocuments(), []);
  const doc = docs.data?.find((d) => d.id === docId) ?? null;
  const [team, setTeam] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<(AccessResult & { kind: "gave" | "took" }) | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [removed, setRemoved] = useState<DeleteResult | null>(null);
  const result = useRef<HTMLParagraphElement>(null);
  const confirmText = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    if (done || removed) result.current?.focus();
  }, [done, removed]);
  useEffect(() => {
    if (confirming) confirmText.current?.focus();
  }, [confirming]);

  if (removed)
    return (
      <>
        <BackLink />
        <header className="page-head">
          <h1 id="view-title">Access</h1>
        </header>
        <div role="status" className="live">
          <p className="result" ref={result} tabIndex={-1}>
            <strong>{removed.message}</strong>
          </p>
        </div>
        <p>
          <a href="#/documents">Back to the documents</a>
        </p>
      </>
    );

  if (docs.loading && !docs.data) return <p className="quiet" role="status">Loading the document.</p>;
  if (docs.error) return <p className="problem-line" role="alert">{docs.error}</p>;
  if (!doc)
    return (
      <>
        <BackLink />
        <header className="page-head">
          <h1 id="view-title">Access</h1>
        </header>
        <p className="problem-line" role="alert">That document does not exist. Go back to the documents list and choose one.</p>
      </>
    );

  const holders = new Set(doc.teams.map((t) => t.id));
  const candidates = teams.filter((t) => !holders.has(t.id));
  const chosen = candidates.find((t) => t.id === team) ?? null;
  const passages = `${doc.passages} ${doc.passages === 1 ? "passage" : "passages"}`;

  async function confirm(e: FormEvent) {
    e.preventDefault();
    if (!chosen || busy || readOnly) return;
    setBusy(true);
    setError(null);
    try {
      const r = await extendAccess(doc!.id, chosen.id);
      setDone({ ...r, kind: "gave" });
      setTeam(null);
      docs.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Access was not changed. Try again.");
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (busy || readOnly) return;
    setBusy(true);
    setError(null);
    try {
      setRemoved(await deleteDocument(doc!.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : "The document was not deleted. Try again.");
      setConfirming(false);
    } finally {
      setBusy(false);
    }
  }
  async function takeBack(teamId: number) {
    if (busy || readOnly) return;
    setBusy(true);
    setError(null);
    try {
      const r = await revokeAccess(doc!.id, teamId);
      setDone({ ...r, kind: "took" });
      docs.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Access was not changed. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <BackLink />
      <header className="page-head">
        <h1 id="view-title">Access</h1>
        <p>
          <strong className="subject">{doc.title}</strong>
          <span className="doc-file"> {passages}, from {doc.source_filename}</span>
        </p>
      </header>

      <div role="status" className="live">
        {done && (
          <p className="result" ref={result} tabIndex={-1}>
            <strong>{done.message}</strong>
            {done.kind === "gave" && <span>A question {done.team.name} was refused for this document is answered when it is asked again.</span>}
          </p>
        )}
      </div>
      {error && <p className="problem-line" role="alert">{error}</p>}

      <section className="block" aria-labelledby="now-title">
        <h2 id="now-title" className="block-title">Who can read it now</h2>
        <ul className="holders">
          {doc.teams.map((t) => (
            <li key={t.id}>
              <span className={"tag" + (t.source === "extension" ? " is-extended" : "")}>{t.name}</span>
              <span className="holder-note">{t.source === "extension" ? "Extended after upload" : "Given at upload"}</span>
              {t.source === "extension" && (
                <button type="button" className="link-btn" disabled={busy || readOnly} onClick={() => takeBack(t.id)} aria-label={`Take back ${t.name}'s access to ${doc.title}`}>
                  Take back
                </button>
              )}
            </li>
          ))}
        </ul>
      </section>

      <form className="grant block" onSubmit={confirm} aria-labelledby="grant-title">
        <h2 id="grant-title" className="block-title">Give another team access</h2>
        {candidates.length === 0 ? (
          <p className="quiet">Every team can already read this document.</p>
        ) : (
          <>
            <fieldset className="plain choices" disabled={busy || readOnly}>
              <legend className="visually-hidden">Team to give access to</legend>
              {candidates.map((t) => (
                <label key={t.id} className="choice">
                  <input type="radio" name="team" checked={team === t.id} onChange={() => setTeam(t.id)} />
                  <span>{t.name}</span>
                </label>
              ))}
            </fieldset>
            <p className="consequence" aria-live="polite">
              {chosen ? (
                <>
                  <strong>{chosen.name}</strong> will be able to read all {passages} of {doc.title}. Keystone will use them to answer {chosen.name}'s questions, starting with the next one {chosen.name} asks. Nothing is copied: {chosen.name} reads the same document.
                </>
              ) : (
                "Choose a team to see what changes."
              )}
            </p>
            <div className="actions">
              <button type="submit" className="btn" disabled={!chosen || busy || readOnly}>
                {chosen ? `Give ${chosen.name} access to this document` : "Choose a team"}
              </button>
              {readOnly && <span className="quiet">This deployment is read-only, so access cannot be changed.</span>}
            </div>
          </>
        )}
      </form>

      <section className="block" aria-labelledby="del-title">
        <h2 id="del-title" className="block-title">Delete this document</h2>
        {!confirming ? (
          <>
            <p className="quiet">Removes it from Keystone for good, with its passages, its upload record and the logged questions whose answers cited it.</p>
            <div className="actions">
              <button type="button" className="btn btn-quiet" disabled={busy || readOnly} onClick={() => setConfirming(true)}>
                Delete this document
              </button>
              {readOnly && <span className="quiet">This deployment is read-only, so documents cannot be deleted.</span>}
            </div>
          </>
        ) : (
          <>
            <p className="consequence" ref={confirmText} tabIndex={-1}>
              <strong>{doc.title}</strong> will be deleted: its {passages}, its upload record and every logged question whose answer cited it. Keystone stops using it at once, and this cannot be undone.
            </p>
            <div className="actions">
              <button type="button" className="btn" disabled={busy} onClick={remove}>
                Delete {doc.title} permanently
              </button>
              <button type="button" className="btn btn-quiet" disabled={busy} onClick={() => setConfirming(false)}>
                Keep it
              </button>
            </div>
          </>
        )}
      </section>
    </>
  );
}

function BackLink() {
  return (
    <p className="back">
      <a href="#/documents">Documents</a>
    </p>
  );
}
