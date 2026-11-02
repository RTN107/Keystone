import { ApiError, tokenStore } from "../api";
import type { Citation } from "./AnswerWithMargin";

const BASE = (import.meta.env.VITE_API_URL ?? "http://localhost:8080").replace(/\/$/, "");

export type Outcome = "answered" | "blocked" | "no_match";
export type Done = {
  outcome: Outcome;
  answer?: string;
  message?: string;
  holding_teams?: string[];
  your_team?: string;
  citations: Citation[];
  truncated?: boolean;
};
export type AskEvent =
  | { type: "waiting"; attempt: number; maxAttempts: number; retryIn: number; message: string }
  | { type: "start"; outcome: Outcome }
  | { type: "token"; text: string }
  | { type: "done"; done: Done }
  | { type: "error"; code: string; message: string; partial: boolean };

// POST /api/ask and read the server-sent events as they arrive. EventSource cannot
// POST, so this reads the response body directly. A refusal before the stream opens
// (rate limit, a missing key) is an ordinary JSON error and is thrown as ApiError.
export async function* askStream(question: string, signal: AbortSignal): AsyncGenerator<AskEvent> {
  let res: Response;
  try {
    res = await fetch(BASE + "/api/ask", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${tokenStore.get() ?? ""}` },
      body: JSON.stringify({ question }),
      signal,
    });
  } catch (e) {
    if (signal.aborted) throw e;
    throw new ApiError(0, "unreachable", "Keystone could not reach the server. Check your connection, or wait a moment if it is waking up, then try again.");
  }
  if (!res.ok || !res.body) {
    const data = await res.json().catch(() => null);
    throw new ApiError(res.status, data?.error?.code ?? "error", data?.error?.message ?? "Something went wrong. Try again in a moment.");
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    let cut: number;
    while ((cut = buf.indexOf("\n\n")) >= 0) {
      const frame = buf.slice(0, cut);
      buf = buf.slice(cut + 2);
      const ev = parseFrame(frame);
      if (ev) yield ev;
    }
  }
}

function parseFrame(frame: string): AskEvent | null {
  let name = "";
  const data: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event: ")) name = line.slice(7);
    else if (line.startsWith("data: ")) data.push(line.slice(6));
  }
  if (!name || !data.length) return null;
  const d = JSON.parse(data.join("\n"));
  switch (name) {
    case "waiting":
      return { type: "waiting", attempt: d.attempt, maxAttempts: d.max_attempts, retryIn: d.retry_in_seconds, message: d.message };
    case "start":
      return { type: "start", outcome: d.outcome };
    case "token":
      return { type: "token", text: d.text };
    case "done":
      return { type: "done", done: { ...d, citations: d.citations ?? [] } };
    case "error":
      return { type: "error", code: d.code, message: d.message, partial: !!d.partial };
  }
  return null;
}
