import type { Citation } from "./AnswerWithMargin";
import type { AskEvent, Done } from "./stream";

// One explicit state for each thing the person can be looking at. There is no state
// that is "loading": the waiting, searching and writing states each say what is
// actually happening.
export type View =
  | { phase: "idle" }
  | { phase: "searching"; question: string }
  | { phase: "waiting"; question: string; attempt: number; maxAttempts: number; retryIn: number; message: string; startedAt: number; firstAt: number }
  | { phase: "writing"; question: string; text: string; startedAt: number }
  | { phase: "answered"; question: string; text: string; citations: Citation[]; truncated: boolean }
  | { phase: "blocked"; question: string; message: string; holdingTeams: string[]; yourTeam: string }
  | { phase: "no_match"; question: string; message: string }
  | { phase: "error"; question: string; code: string; message: string; partialText: string };

export type Action = { type: "ask"; question: string } | { type: "event"; ev: AskEvent } | { type: "fail"; code: string; message: string } | { type: "stopped" } | { type: "clear" };

export function reduce(v: View, a: Action, now: number = Date.now()): View {
  switch (a.type) {
    case "ask":
      return { phase: "searching", question: a.question };
    case "clear":
    case "stopped":
      return { phase: "idle" };
    case "fail":
      return { phase: "error", question: "question" in v ? v.question : "", code: a.code, message: a.message, partialText: v.phase === "writing" ? v.text : "" };
    case "event": {
      const ev = a.ev;
      const question = "question" in v ? v.question : "";
      switch (ev.type) {
        case "waiting":
          return {
            phase: "waiting",
            question,
            attempt: ev.attempt,
            maxAttempts: ev.maxAttempts,
            retryIn: ev.retryIn,
            message: ev.message,
            startedAt: now,
            firstAt: v.phase === "waiting" ? v.firstAt : now,
          };
        case "start":
          return ev.outcome === "answered" ? { phase: "writing", question, text: "", startedAt: now } : v;
        case "token":
          return v.phase === "writing" ? { ...v, text: v.text + ev.text } : { phase: "writing", question, text: ev.text, startedAt: now };
        case "done":
          return finish(question, ev.done, v);
        case "error":
          return { phase: "error", question, code: ev.code, message: ev.message, partialText: ev.partial && v.phase === "writing" ? v.text : "" };
      }
    }
  }
  return v;
}

function finish(question: string, d: Done, v: View): View {
  if (d.outcome === "answered") return { phase: "answered", question, text: d.answer ?? (v.phase === "writing" ? v.text : ""), citations: d.citations, truncated: !!d.truncated };
  if (d.outcome === "blocked") return { phase: "blocked", question, message: d.message ?? "", holdingTeams: d.holding_teams ?? [], yourTeam: d.your_team ?? "" };
  return { phase: "no_match", question, message: d.message ?? "" };
}

// Two questions are the same question if they differ only in case and spacing,
// exactly as the server's query cache sees them.
export const sameQuestion = (a: string, b: string) => norm(a) === norm(b);
const norm = (s: string) => s.trim().toLowerCase().split(/\s+/).join(" ");
