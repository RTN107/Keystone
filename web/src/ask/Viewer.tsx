import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { api, ApiError } from "../api";
import type { Citation } from "./AnswerWithMargin";
import { splitPassage } from "./AnswerWithMargin";
import { cells, overlaps, parseDocument, tableRows, type DocBlock, type Line } from "./docText";
import "./viewer.css";

type Doc = { id: number; title: string; content: string };
type Load = { s: "loading" } | { s: "ready"; doc: Doc } | { s: "gone" } | { s: "error"; message: string };
const cache = new Map<number, Doc>();

// The source viewer: the whole document, with the cited passage marked and brought into
// view. It is a modal dialog, so the page behind is inert, Tab stays inside, and Escape
// closes it. Closing returns focus to the note that opened it.
export function Viewer({ citation, from, onClose }: { citation: Citation; from: HTMLElement | null; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const [load, setLoad] = useState<Load>(() => (cache.has(citation.document_id) ? { s: "ready", doc: cache.get(citation.document_id)! } : { s: "loading" }));
  const [attempt, setAttempt] = useState(0);
  const { section } = splitPassage(citation.passage);

  useLayoutEffect(() => {
    const d = dialog.current;
    if (d && !d.open) d.showModal();
  }, []);

  useEffect(() => {
    if (cache.has(citation.document_id)) return;
    let live = true;
    setLoad({ s: "loading" });
    api<Doc>(`/api/documents/${citation.document_id}`)
      .then((doc) => {
        cache.set(doc.id, doc);
        if (live) setLoad({ s: "ready", doc });
      })
      .catch((e) => {
        if (!live) return;
        setLoad(e instanceof ApiError && e.status === 404 ? { s: "gone" } : { s: "error", message: e instanceof Error ? e.message : "Something went wrong." });
      });
    return () => {
      live = false;
    };
  }, [citation.document_id, attempt]);

  // Bring the cited passage into view once the document is on screen and the fonts have settled.
  useEffect(() => {
    if (load.s !== "ready") return;
    const place = () => {
      const band = body.current?.querySelector<HTMLElement>(".cited-band");
      const box = body.current;
      // leave a little of the section above in view, so the passage has its context
      if (band && box) box.scrollTop += band.getBoundingClientRect().top - box.getBoundingClientRect().top - 96;
    };
    place();
    void document.fonts?.ready.then(place);
  }, [load]);

  function close() {
    dialog.current?.close();
  }
  return (
    <dialog
      ref={dialog}
      className="viewer"
      aria-labelledby="viewer-title"
      onClose={() => {
        onClose();
        from?.focus();
      }}
      onClick={(e) => e.target === dialog.current && close()} // a click on the backdrop
    >
      <header className="viewer-head">
        <div>
          <h2 id="viewer-title">{load.s === "ready" ? load.doc.title : citation.document_title}</h2>
          <p className="viewer-sub">
            Source {citation.n}
            {section && <> &middot; {section}</>}
          </p>
        </div>
        <button type="button" className="btn btn-quiet viewer-close" onClick={close} autoFocus>
          Close
        </button>
      </header>
      <div className="viewer-body" ref={body} tabIndex={0} role="region" aria-label="Document text">
        {load.s === "loading" && <p className="viewer-note">Opening the document.</p>}
        {load.s === "gone" && <p className="viewer-note">That document is not available to your team.</p>}
        {load.s === "error" && (
          <div className="viewer-note" role="alert">
            <p>{load.message}</p>
            <button type="button" className="btn btn-quiet" onClick={() => setAttempt((a) => a + 1)}>
              Try again
            </button>
          </div>
        )}
        {load.s === "ready" && <DocView content={load.doc.content} start={citation.char_start} end={citation.char_end} />}
      </div>
    </dialog>
  );
}

function DocView({ content, start, end }: { content: string; start: number; end: number }) {
  const blocks = parseDocument(content);
  const out: ReactNode[] = [];
  for (let i = 0; i < blocks.length; ) {
    if (!overlaps(blocks[i], start, end)) {
      out.push(<Block key={i} b={blocks[i]} />);
      i++;
      continue;
    }
    // Consecutive cited blocks share one band, so the marked passage reads as one thing.
    const from = i;
    const inner: ReactNode[] = [];
    while (i < blocks.length && overlaps(blocks[i], start, end)) inner.push(<Block key={i} b={blocks[i]} />), i++;
    out.push(
      <div key={"band" + from} className="cited-band" aria-label="The cited passage" role="group">
        {inner}
      </div>,
    );
  }
  return <article className="doc">{out}</article>;
}

const Lines = ({ ls }: { ls: Line[] }) => (
  <>
    {ls.map((l, i) => (
      <span key={l.start} className="doc-line">
        {l.text}
        {i < ls.length - 1 && <br />}
      </span>
    ))}
  </>
);

function Block({ b }: { b: DocBlock }) {
  switch (b.kind) {
    case "h1":
      return <h3 className="doc-h1">{b.line.text.replace(/^#\s+/, "")}</h3>;
    case "h2":
      return <h4 className="doc-h2">{b.line.text.replace(/^##\s+/, "")}</h4>;
    case "text":
      return (
        <p className="doc-p">
          <Lines ls={b.lines} />
        </p>
      );
    case "list": {
      const Tag = b.ordered ? "ol" : "ul";
      return (
        <Tag className="doc-list">
          {b.lines.map((l) => (
            <li key={l.start}>{l.text.replace(/^(\d+\.|[-*])\s+/, "")}</li>
          ))}
        </Tag>
      );
    }
    case "table": {
      const rows = tableRows(b.rows);
      return (
        <div className="doc-table-wrap">
          <table className="doc-table">
            <thead>
              <tr>
                {cells(rows[0].text).map((c, i) => (
                  <th key={i}>{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.slice(1).map((r) => (
                <tr key={r.start}>
                  {cells(r.text).map((c, i) => (
                    <td key={i}>{c}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    }
  }
}
