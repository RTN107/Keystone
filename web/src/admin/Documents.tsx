import { useRef, useState, type FormEvent } from "react";
import { type Doc, type TeamWithMembers, type Uploaded, listDocuments, uploadDocument } from "./adminApi";
import { useLoad, when } from "./useLoad";

const TYPES = ".pdf,.md,.markdown,.txt,application/pdf,text/markdown,text/plain";

export function Documents({ teams, readOnly, embeddingsOn, justAdded, onAdded }: {
  teams: TeamWithMembers[];
  readOnly: boolean;
  embeddingsOn: boolean;
  justAdded: number | null;
  onAdded: (id: number) => void;
}) {
  const docs = useLoad(() => (embeddingsOn ? listDocuments() : Promise.resolve([] as Doc[])), [embeddingsOn]);
  return (
    <>
      <header className="page-head">
        <h1 id="view-title">Documents</h1>
        <p>What Keystone answers from, and which teams can read each document.</p>
      </header>
      {!embeddingsOn ? (
        <p className="note-box" role="status">
          Documents cannot be added on this deployment: it has no embeddings key. Set VOYAGE_API_KEY and restart the server.
        </p>
      ) : (
        <Upload teams={teams} readOnly={readOnly} onDone={(u) => { onAdded(u.document_id); docs.reload(); }} />
      )}
      <section aria-labelledby="list-title" className="block">
        <h2 id="list-title" className="block-title">
          {docs.data ? `${docs.data.length} ${docs.data.length === 1 ? "document" : "documents"}` : "Documents"}
        </h2>
        {docs.error && (
          <p className="problem-line" role="alert">
            {docs.error}
          </p>
        )}
        {docs.loading && !docs.data && <p className="quiet" role="status">Loading the documents.</p>}
        {docs.data && docs.data.length === 0 && <p className="quiet">No documents yet. Add the first one above.</p>}
        {docs.data && docs.data.length > 0 && (
          <table className="table docs">
            <thead>
              <tr>
                <th scope="col">Document</th>
                <th scope="col">Readable by</th>
                <th scope="col" className="num">Passages</th>
                <th scope="col">Added</th>
                <th scope="col"><span className="visually-hidden">Actions</span></th>
              </tr>
            </thead>
            <tbody>
              {docs.data.map((d) => (
                <tr key={d.id} className={d.id === justAdded ? "is-new" : undefined}>
                  <th scope="row" data-label="Document">
                    <span className="doc-title">{d.title}</span>
                    <span className="doc-file">{d.source_filename}</span>
                  </th>
                  <td data-label="Readable by">
                    <span className="tags">
                      {d.teams.map((t) => (
                        <span key={t.id} className={"tag" + (t.source === "extension" ? " is-extended" : "")}>
                          {t.name}
                          {t.source === "extension" && <span className="tag-note"> (extended)</span>}
                        </span>
                      ))}
                    </span>
                  </td>
                  <td className="num" data-label="Passages">{d.passages}</td>
                  <td data-label="Added">
                    {when(d.created_at)}
                    {d.ingested_by && <span className="doc-file"> by {d.ingested_by}</span>}
                  </td>
                  <td className="act">
                    <a className="row-link" href={`#/documents/${d.id}/access`} aria-label={`Manage access to ${d.title}`}>
                      Access
                    </a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </>
  );
}

function Upload({ teams, readOnly, onDone }: { teams: TeamWithMembers[]; readOnly: boolean; onDone: (u: Uploaded) => void }) {
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [picked, setPicked] = useState<number[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Uploaded | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const ready = !!file && picked.length > 0 && !busy && !readOnly;
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!file || !ready) return;
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const u = await uploadDocument(file, picked, title);
      setResult(u);
      setFile(null);
      setTitle("");
      setPicked([]);
      if (fileInput.current) fileInput.current.value = "";
      onDone(u);
    } catch (err) {
      setError(err instanceof Error ? err.message : "The upload failed. Nothing was saved.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="block upload" onSubmit={submit} aria-labelledby="upload-title">
      <h2 id="upload-title" className="block-title">Add a document</h2>
      <fieldset disabled={readOnly || busy} className="plain">
        <label className="field">
          <span>File</span>
          <input ref={fileInput} className="input file" type="file" accept={TYPES} onChange={(e) => setFile(e.target.files?.[0] ?? null)} aria-describedby="file-hint" />
          <small id="file-hint" className="hint">PDF, Markdown or text, up to 5 MB.</small>
        </label>
        <label className="field">
          <span>Title <em className="optional">optional</em></span>
          <input className="input" value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} aria-describedby="title-hint" />
          <small id="title-hint" className="hint">Left empty, it is the document's first heading.</small>
        </label>
        <fieldset className="plain choices">
          <legend>Who can read it</legend>
          {teams.map((t) => (
            <label key={t.id} className="choice">
              <input type="checkbox" checked={picked.includes(t.id)} onChange={(e) => setPicked((p) => (e.target.checked ? [...p, t.id] : p.filter((x) => x !== t.id)))} />
              <span>{t.name}</span>
            </label>
          ))}
        </fieldset>
      </fieldset>
      <div className="actions">
        <button type="submit" className="btn" disabled={!ready}>
          {busy ? "Embedding" : "Upload and embed"}
        </button>
        {readOnly && <span className="quiet">This deployment is read-only, so documents cannot be added.</span>}
        {!readOnly && !busy && !ready && !result && <span className="quiet">{!file ? "Choose a file" : "Choose at least one team"} to continue.</span>}
      </div>
      <div aria-live="polite">
        {busy && <p className="quiet status">Embedding the document. On a free embeddings plan this can take up to a minute.</p>}
        {error && <p className="problem-line" role="alert">{error}</p>}
        {result && (
          <p className="result">
            <strong>{result.message}</strong>
            <span>{result.title}. Readable by {result.teams.join(", ")}.</span>
          </p>
        )}
      </div>
    </form>
  );
}
