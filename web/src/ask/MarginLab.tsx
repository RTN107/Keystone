import { useCallback, useEffect, useRef, useState } from "react";
import { AnswerWithMargin, COMPACT_REM, type Citation } from "./AnswerWithMargin";
import p1 from "../fixtures/p1-breach.json";
import freeze from "../fixtures/deploy-freeze.json";
import approval from "../fixtures/discount-approval.json";
import retainer from "../fixtures/retainer-discount-granted.json";

type Fixture = { question: string; done: { answer: string; citations: Citation[] } };

// Stress cases built from REAL passages (taken from the captured answers), arranged
// to break the layout on purpose.
const real: Record<string, Fixture> = { "p1-breach": p1, "deploy-freeze": freeze, "discount-approval": approval, "retainer-discount-granted": retainer };
const pool = [...retainer.done.citations, ...p1.done.citations];
const renum = (idx: number[]): Citation[] => idx.map((k, i) => ({ ...pool[k], n: i + 1 }));

const stress: Record<string, Fixture> = {
  // Three different sources cited in one short paragraph, then one in the next.
  "stress-three-in-one-paragraph": {
    question: "Stress: one short paragraph carries three citations",
    done: {
      answer:
        "Retainer discounts are capped by term, applied to labour only, and shown as a separate line [1][2][3].\n\nA client who leaves early repays what they received [2].\n\nRenewals earn the matching tier [4].",
      citations: renum([0, 1, 2, 3]),
    },
  },
  // One source supports the first paragraph and, much later, the last: a citation
  // that spans a paragraph boundary, with other notes between.
  "stress-one-source-across-paragraphs": {
    question: "Stress: source 1 is cited at the top and again at the bottom",
    done: {
      answer:
        "The tier depends on the term [1].\n\nAnything above ten percent goes to the Head of Sales [2].\n\nThe worked example shows the arithmetic [3].\n\nSo the answer is the same one the first paragraph gave [1].",
      citations: renum([0, 1, 2]),
    },
  },
};
const all: Record<string, Fixture> = { ...real, ...stress };
const widths = [1280, 1024, 820, 720, 600, 390];

type Metrics = { compact: boolean; marginNotes: number; inlineNotes: number; inlineBlocks: string; overlaps: number; maxPush: number; marginBelowText: number; hScroll: boolean };

function measureLab(root: HTMLElement): Metrics {
  const frame = root.querySelector<HTMLElement>(".answer-frame")!;
  const compact = frame.dataset.compact === "true";
  const shown = [...frame.querySelectorAll<HTMLElement>(".note:not(.is-shadow)")];
  const rects = shown.map((n) => n.getBoundingClientRect());
  let overlaps = 0;
  for (let i = 0; i < rects.length; i++) for (let j = i + 1; j < rects.length; j++) {
    const a = rects[i], b = rects[j];
    if (a.left < b.right && b.left < a.right && a.top < b.bottom - 0.5 && b.top < a.bottom - 0.5) overlaps++;
  }
  // How far each margin note sits below the marker that cites it.
  const pushes: number[] = [];
  const margin = shown.filter((n) => n.classList.contains("is-margin"));
  for (const n of margin) {
    const marker = frame.querySelector<HTMLElement>(`[data-cite="${n.dataset.note}"]`);
    if (marker) pushes.push(n.getBoundingClientRect().top - marker.getBoundingClientRect().top);
  }
  const text = frame.querySelector<HTMLElement>(".answer-text")!.getBoundingClientRect();
  const lowest = Math.max(0, ...margin.map((n) => n.getBoundingClientRect().bottom));
  return {
    compact,
    marginNotes: margin.length,
    inlineNotes: shown.length - margin.length,
    inlineBlocks: frame.dataset.inlineBlocks ?? "",
    overlaps,
    maxPush: Math.round(Math.max(0, ...pushes)),
    marginBelowText: margin.length ? Math.round(Math.max(0, lowest - text.bottom)) : 0,
    hScroll: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  };
}

// Development page: /?lab=margin&fixture=p1-breach&width=1024
export function MarginLab() {
  const params = new URLSearchParams(location.search);
  const [name, setName] = useState(params.get("fixture") && all[params.get("fixture")!] ? params.get("fixture")! : "p1-breach");
  const [width, setWidth] = useState(Number(params.get("width")) || 1280);
  const [m, setM] = useState<Metrics | null>(null);
  const holder = useRef<HTMLDivElement>(null);

  const update = useCallback(() => {
    if (!holder.current) return;
    const metrics = measureLab(holder.current);
    setM(metrics);
    (window as unknown as { __lab: Metrics }).__lab = metrics;
  }, []);
  // Re-measure whenever anything in the holder changes size, so the numbers describe
  // the settled layout and not the first paint.
  useEffect(() => {
    const el = holder.current;
    if (!el) return;
    let timer = 0;
    const later = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(update, 150);
    };
    const ro = new ResizeObserver(later);
    ro.observe(el);
    el.querySelectorAll(".answer-text, .margin, .note").forEach((n) => ro.observe(n));
    later();
    return () => {
      window.clearTimeout(timer);
      ro.disconnect();
    };
  }, [name, width, update, m?.compact]);

  const f = all[name];
  return (
    <div style={{ background: "var(--wash)", minHeight: "100vh", padding: "1rem" }}>
      <div style={{ fontFamily: "var(--font-system)", fontSize: 13, display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center", marginBottom: 12 }}>
        <select value={name} onChange={(e) => setName(e.target.value)}>
          {Object.keys(all).map((k) => (
            <option key={k}>{k}</option>
          ))}
        </select>
        {widths.map((w) => (
          <button key={w} onClick={() => setWidth(w)} style={{ fontWeight: w === width ? 700 : 400 }}>
            {w}
          </button>
        ))}
        <span data-testid="metrics">
          {m && `${m.compact ? "all inline (below " + COMPACT_REM + "rem)" : "margin"} | margin notes ${m.marginNotes} | inline notes ${m.inlineNotes} (paragraphs ${m.inlineBlocks || "none"}) | overlaps ${m.overlaps} | push max ${m.maxPush}px | margin past text ${m.marginBelowText}px | h-scroll ${m.hScroll}`}
        </span>
      </div>
      <div ref={holder} style={{ width, maxWidth: "100%", background: "var(--sheet)", padding: "1.5rem", boxSizing: "border-box" }}>
        <p style={{ fontFamily: "var(--font-system)", fontWeight: 650, margin: "0 0 1rem", fontSize: "1.1rem", color: "var(--ink)" }}>{f.question}</p>
        <AnswerWithMargin answer={f.done.answer} citations={f.done.citations} />
      </div>
    </div>
  );
}
