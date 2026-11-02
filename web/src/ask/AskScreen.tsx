import { useCallback, useEffect, useReducer, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { ApiError, type Employee } from "../api";
import { AnswerWithMargin, type Citation } from "./AnswerWithMargin";
import { askStream } from "./stream";
import { reduce, sameQuestion, type View } from "./state";
import { Slot } from "./Slot";
import { Waiting } from "./Waiting";
import "./ask-screen.css";

const MAX = 500;

function statusText(v: View): string {
  switch (v.phase) {
    case "answered":
      return `Answer ready, with ${v.citations.length} ${v.citations.length === 1 ? "source" : "sources"}.`;
    case "blocked":
      return "This question is outside your team's access.";
    case "no_match":
      return "No answer was found in the documents your team can read.";
    case "error":
      return "The question could not be answered.";
    default:
      return "";
  }
}

export function AskScreen({ me, canAsk, offReason, onOpen }: {
  me: Employee;
  canAsk: boolean;
  offReason: string;
  onOpen: (c: Citation, from: HTMLElement) => void;
}) {
  const [view, dispatch] = useReducer((v: View, a: Parameters<typeof reduce>[1]) => reduce(v, a), { phase: "idle" } as View);
  const [text, setText] = useState("");
  const abort = useRef<AbortController | null>(null);
  // The client remembers a blocked question. Asking that same question again keeps the empty slot
  // in the margin while the answer is found, and if the answer comes back the slot is completed.
  const [memo, setMemo] = useState<{ question: string; teams: string[]; answered?: boolean } | null>(null);
  const [reopen, setReopen] = useState(false);
  const memoRef = useRef(memo);
  memoRef.current = memo;
  const area = useRef<HTMLTextAreaElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const busy = view.phase === "searching" || view.phase === "waiting" || view.phase === "writing";
  const team = me.team?.name ?? "your team";

  const run = useCallback(async (question: string) => {
    abort.current?.abort();
    const ac = new AbortController();
    abort.current = ac;
    const again = !!memoRef.current && !memoRef.current.answered && sameQuestion(memoRef.current.question, question);
    setReopen(again);
    if (!again) setMemo(null);
    dispatch({ type: "ask", question });
    let finished = false;
    try {
      for await (const ev of askStream(question, ac.signal)) {
        if (ev.type === "done" || ev.type === "error") finished = true;
        dispatch({ type: "event", ev });
      }
      if (!finished) dispatch({ type: "fail", code: "cut_off", message: "The connection ended before the answer was complete. Nothing was lost. Ask again in a moment." });
    } catch (e) {
      if (ac.signal.aborted) return;
      dispatch({ type: "fail", code: e instanceof ApiError ? e.code : "error", message: e instanceof Error ? e.message : "Something went wrong. Try again in a moment." });
    }
  }, []);
  useEffect(() => () => abort.current?.abort(), []);
  useEffect(() => {
    if (view.phase === "blocked") setMemo({ question: view.question, teams: view.holdingTeams });
    else if (view.phase === "answered") setMemo((m) => (m ? { ...m, answered: true } : m)); // consumed: its teams stay until the next question so the fill can draw the slot once
    else if (view.phase === "no_match" || view.phase === "error" || view.phase === "idle") setReopen(false);
  }, [view]);

  // Grow with the text, up to five lines.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = Math.min(el.scrollHeight, 5 * 26 + 20) + "px";
  }, [text]);

  function submit(e?: FormEvent) {
    e?.preventDefault();
    const q = text.trim();
    if (!q || busy || !canAsk) return;
    setText("");
    void run(q);
  }
  function onKey(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      submit();
    }
  }
  function stop() {
    abort.current?.abort();
    dispatch({ type: "stopped" });
    area.current?.focus();
  }

  return (
    <main id="main" className="ask" aria-labelledby="ask-label" tabIndex={-1}>
      <form className="composer" onSubmit={submit}>
        <label id="ask-label" htmlFor="question" className="composer-label">
          Ask a question
        </label>
        <div className="composer-box" onClick={(e) => e.target === e.currentTarget && area.current?.focus()}>
          <textarea
            id="question"
            ref={area}
            rows={1}
            value={text}
            maxLength={MAX}
            disabled={!canAsk}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKey}
            aria-describedby="ask-hint"
            enterKeyHint="send"
            spellCheck
          />
          {busy ? (
            <button type="button" className="btn btn-quiet" onClick={stop}>
              Stop
            </button>
          ) : (
            <button type="submit" className="btn" disabled={!canAsk || !text.trim()}>
              Ask
            </button>
          )}
        </div>
        <p id="ask-hint" className="hint">
          {canAsk ? `Answers come only from documents ${team} can read.` : offReason}
          {text.length > MAX - 80 && <span className="count tnum"> {text.length} of {MAX}</span>}
        </p>
      </form>

      <p className="visually-hidden" role="status">
        {statusText(view)}
      </p>

      <section className="result" aria-label="Answer" aria-busy={busy}>
        {view.phase !== "idle" && (
          <h2 className="question" ref={heading} tabIndex={-1}>
            {view.question}
          </h2>
        )}
        <Body v={view} team={team} onOpen={onOpen} onRetry={() => view.phase === "error" && void run(view.question)} onAskAgain={() => {
          if (!("question" in view)) return;
          void run(view.question);
          // The button that was pressed is replaced while the question is asked again; keep the place.
          heading.current?.focus({ preventScroll: true });
        }} held={reopen && memo ? memo.teams : null} reopened={reopen} />
      </section>
    </main>
  );
}

function Body({ v, team, onOpen, onRetry, onAskAgain, held, reopened }: {
  v: View;
  team: string;
  onOpen: (c: Citation, from: HTMLElement) => void;
  onRetry: () => void;
  onAskAgain: () => void;
  // The teams holding a blocked question that is being asked again, so its slot can stay while it is answered.
  held: string[] | null;
  reopened: boolean;
}) {
  const slot = held ? <Slot teams={held} variant="margin" /> : null;
  switch (v.phase) {
    case "idle":
      return null;
    case "searching":
      return (
        <SlotFrame slot={held}>
          <p className="quiet-status" key="s">
            Reading what {team} can read.
          </p>
        </SlotFrame>
      );
    case "waiting":
      return (
        <SlotFrame slot={held}>
          <Waiting v={v} />
        </SlotFrame>
      );
    case "writing":
      return v.text ? (
        <AnswerWithMargin answer={v.text} citations={[]} slot={slot} />
      ) : (
        <SlotFrame slot={held}>
          <Writing startedAt={v.startedAt} />
        </SlotFrame>
      );
    case "answered":
      return (
        <>
          <AnswerWithMargin answer={v.text} citations={v.citations} onOpen={onOpen} animate fill={reopened && held ? { teams: held } : undefined} />
          {v.truncated && <p className="quiet-status">The answer reached its length limit and may be incomplete.</p>}
        </>
      );
    case "blocked":
      return <Blocked v={v} reopened={reopened} onAskAgain={onAskAgain} />;
    case "no_match":
      return <p className="plain-message">{v.message}</p>;
    case "error":
      // A limit, not a fault: said plainly in the same voice as the other plain messages.
      if (["resting", "rate_limited", "busy"].includes(v.code)) return <p className="plain-message">{v.message}</p>;
      return (
        <div className="problem" role="alert">
          {v.partialText && (
            <div className="problem-partial">
              <AnswerWithMargin answer={v.partialText} citations={[]} />
            </div>
          )}
          <p>{v.message}</p>
          {["answer_failed", "cut_off", "embeddings_busy", "embeddings_unreachable", "unreachable", "internal"].includes(v.code) && (
            <button type="button" className="btn btn-quiet" onClick={onRetry}>
              Ask again
            </button>
          )}
        </div>
      );
  }
}

// After the search the model needs a moment, which varies from under a second to several.
// Say what is happening, and say so honestly when it is taking longer than usual.
function Writing({ startedAt }: { startedAt: number }) {
  const [late, setLate] = useState(false);
  useEffect(() => {
    const id = window.setTimeout(() => setLate(true), 5000 - Math.max(0, Date.now() - startedAt));
    return () => window.clearTimeout(id);
  }, [startedAt]);
  return (
    <p className="quiet-status" key="w">
      Found what your team's documents say. Writing the answer.
      {late && <span className="late"> This is taking a little longer than usual.</span>}
    </p>
  );
}

// A frame that keeps the sources margin (and its empty slot) beside whatever is in the main column.
function SlotFrame({ slot, children }: { slot: string[] | null; children: React.ReactNode }) {
  if (!slot) return <>{children}</>;
  return (
    <div className="slot-cq">
      <div className="slot-frame">
        <div className="slot-main">{children}</div>
        <Slot teams={slot} variant="margin" />
      </div>
    </div>
  );
}

// The refusal is the server's own static text. It is shown in the system voice, with its first
// sentence (the verdict) set apart, beside the empty slot where a source would have been.
function Blocked({ v, reopened, onAskAgain }: { v: Extract<View, { phase: "blocked" }>; reopened: boolean; onAskAgain: () => void }) {
  const cut = v.message.indexOf(". ");
  const verdict = cut > 0 ? v.message.slice(0, cut + 1) : v.message;
  const rest = cut > 0 ? v.message.slice(cut + 2) : "";
  return (
    <div className="slot-cq">
      <div className="slot-frame is-blocked">
        <p className="blocked-message">
          <strong>{verdict}</strong> {rest}
        </p>
        <Slot teams={v.holdingTeams} variant="margin" />
        <div className="blocked-actions">
          <button type="button" className="btn btn-quiet" onClick={onAskAgain}>
            Ask again
          </button>
          {reopened && <span className="blocked-checked">Checked again. Still closed to your team.</span>}
        </div>
      </div>
    </div>
  );
}
