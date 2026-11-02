import { plainExcerpt } from "./docText";
import { Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { firstBlockOf, parseAnswer, type Block, type Inline } from "./parse";
import { placeNotes } from "./layout";
import { Slot } from "./Slot";
import "./ask.css";

export type Citation = {
  n: number;
  chunk_id: number;
  document_id: number;
  document_title: string;
  passage: string;
  char_start: number;
  char_end: number;
};

// The layout has exactly two states, and both show the excerpt:
//   margin  a note sits beside the paragraph that first cites it;
//   inline  a note sits directly under that paragraph.
// A viewer narrower than COMPACT_BELOW gets inline for every paragraph. In a wide
// viewer, a paragraph goes inline when its notes cannot sit beside it: when stacking
// would push any of them further than PUSH_LIMIT from the marker that cites it.
// Defined in rem, like the layout it protects: margin mode needs a text column of about
// 28rem beside the 17rem margin and the gutter, so below 46rem everything goes inline.
export const COMPACT_REM = 46;
const compactBelow = () => COMPACT_REM * parseFloat(getComputedStyle(document.documentElement).fontSize || "16");
export const PUSH_LIMIT = 64;
const GAP = 10; // space kept between two stacked margin notes
const NUDGE = 3; // a note's first line sits level with the marker's line

type Layout = { compact: boolean; inline: ReadonlySet<number>; tops: Record<number, number>; height: number };
const EMPTY: Layout = { compact: false, inline: new Set(), tops: {}, height: 0 };

// "## When a P1 breaches its SLA\n\nbody..." becomes a section name and its text.
export function splitPassage(passage: string): { section: string | null; body: string } {
  const m = /^#{1,6}\s+([^\n]+)\n+([\s\S]*)$/.exec(passage);
  return m ? { section: m[1].trim(), body: plainExcerpt(m[2].trim()) } : { section: null, body: plainExcerpt(passage.trim()) };
}

type Link = { active: number | null; setActive: (n: number | null) => void; pin: (n: number) => void; known: Set<number> };

// A marker stays with the word before it and the punctuation after it. It is a button, so the browser
// would otherwise be free to break on either side and leave a marker or a full stop alone on a line.
const LEAD = /^[.,;:!?)\]]+/;
const TAIL = /\S+$/;
function Runs({ runs, link }: { runs: Inline[]; link: Link }) {
  const lead = (i: number) => {
    const r = runs[i];
    return r?.kind === "text" ? (LEAD.exec(r.text)?.[0] ?? "") : "";
  };
  const tail = (i: number) => {
    const r = runs[i];
    if (r?.kind !== "text") return "";
    const rest = r.text.slice(runs[i - 1]?.kind === "cite" ? lead(i).length : 0);
    return TAIL.exec(rest)?.[0] ?? "";
  };
  return (
    <>
      {runs.map((r, i) => {
        if (r.kind === "text") {
          const from = runs[i - 1]?.kind === "cite" ? lead(i).length : 0;
          const to = runs[i + 1]?.kind === "cite" ? r.text.length - tail(i).length : r.text.length;
          return <Fragment key={i}>{r.text.slice(from, Math.max(from, to))}</Fragment>;
        }
        const before = runs[i - 1]?.kind === "text" ? tail(i - 1) : "";
        const after = lead(i + 1);
        // While the answer streams the sources are not known yet: the number is shown
        // quietly, and becomes a control when the citations arrive.
        const marker = !link.known.has(r.n) ? (
          <span className="mark is-pending" data-cite={r.n}>
            {r.n}
          </span>
        ) : (
          <button
            type="button"
            className={"mark" + (link.active === r.n ? " is-active" : "")}
            data-cite={r.n}
            aria-label={`Source ${r.n}`}
            onMouseEnter={() => link.setActive(r.n)}
            onMouseLeave={() => link.setActive(null)}
            onFocus={() => link.setActive(r.n)}
            onBlur={() => link.setActive(null)}
            onClick={() => link.pin(r.n)}
          >
            {r.n}
          </button>
        );
        return (
          <span key={i} className="glue">
            {before}
            {marker}
            {after}
          </span>
        );
      })}
    </>
  );
}

function BlockView({ block, link }: { block: Block; link: Link }) {
  if (block.kind === "p")
    return (
      <p className="block">
        <Runs runs={block.inline} link={link} />
      </p>
    );
  const Tag = block.kind;
  return (
    <Tag className="block list">
      {block.items.map((it, i) => (
        <li key={i}>
          <Runs runs={it} link={link} />
        </li>
      ))}
    </Tag>
  );
}

function Note({ c, index, link, variant, style, onOpen, refCb }: {
  c: Citation;
  index: number;
  link: Link;
  variant: "margin" | "inline" | "shadow";
  style?: React.CSSProperties;
  onOpen?: (c: Citation, from: HTMLElement) => void;
  refCb?: (el: HTMLElement | null) => void;
}) {
  const { section, body } = splitPassage(c.passage);
  const shadow = variant === "shadow"; // laid out so it can be measured, not shown
  return (
    <aside
      ref={refCb}
      className={`note is-${variant}` + (link.active === c.n ? " is-active" : "")}
      style={{ ...style, ["--i" as string]: index }}
      data-note={variant === "inline" ? undefined : c.n}
      data-inline-note={variant === "inline" ? c.n : undefined}
      aria-hidden={shadow || undefined}
      onMouseEnter={() => !shadow && link.setActive(c.n)}
      onMouseLeave={() => !shadow && link.setActive(null)}
    >
      <button
        type="button"
        className="note-top"
        data-open={shadow ? undefined : c.n}
        tabIndex={shadow ? -1 : 0}
        aria-label={`Source ${c.n}: ${c.document_title}${section ? ", " + section : ""}. Open the document.`}
        onClick={(e) => onOpen?.(c, e.currentTarget)}
        onFocus={() => link.setActive(c.n)}
        onBlur={() => link.setActive(null)}
      >
        <span className="note-head">
          <span className="note-n">{c.n}</span>
          <span className="note-doc">{c.document_title}</span>
        </span>
        {section && <span className="note-section">{section}</span>}
      </button>
      <div className="note-body-wrap">
        <p className="note-body">{body}</p>
      </div>
    </aside>
  );
}

export function AnswerWithMargin({ answer, citations, onOpen, animate = false, slot, fill }: {
  answer: string;
  citations: Citation[];
  onOpen?: (c: Citation, from: HTMLElement) => void;
  // When true, the notes ease in as the citations arrive (the margin fills after the prose).
  animate?: boolean;
  // A blocked question is being asked again: the empty slot stays in the margin until the answer lands.
  slot?: ReactNode;
  // The same question has just been answered after being blocked: the slot completes into the first
  // note. Only the teams are needed, to draw the slot one last time while it fills.
  fill?: { teams: string[] };
}) {
  const frame = useRef<HTMLDivElement>(null);
  const text = useRef<HTMLDivElement>(null);
  const noteEls = useRef(new Map<number, HTMLElement>());
  const [hover, setHover] = useState<number | null>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const [layout, setLayout] = useState<Layout>(EMPTY);
  const [slotDone, setSlotDone] = useState(false);
  // Reduced motion gets a plain crossfade of the notes; the slot is not drawn a last time.
  const slotFills = !!fill && !slotDone && typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: no-preference)").matches;

  const blocks = useMemo(() => parseAnswer(answer), [answer]);
  const first = useMemo(() => firstBlockOf(blocks), [blocks]);
  const ordered = useMemo(() => [...citations].sort((a, b) => (first.get(a.n) ?? 99) - (first.get(b.n) ?? 99) || a.n - b.n), [citations, first]);
  const known = useMemo(() => new Set(citations.map((c) => c.n)), [citations]);

  // The sticky set of paragraphs that went inline belongs to one answer at one width.
  const sticky = useRef({ key: "", inline: new Set<number>() });

  const measure = useCallback(() => {
    const f = frame.current;
    const t = text.current;
    if (!f || !t) return;
    const key = `${answer.length}:${citations.length}:${Math.round(f.clientWidth / 24)}`;
    if (sticky.current.key !== key) sticky.current = { key, inline: new Set() };
    if (f.clientWidth < compactBelow()) {
      setLayout((l) => (l.compact ? l : { ...EMPTY, compact: true }));
      return;
    }
    const base = t.getBoundingClientRect().top;
    const tops: Record<number, number> = {};
    let floor = Number.NEGATIVE_INFINITY;
    let bottom = t.offsetHeight;
    for (let b = 0; b < blocks.length; b++) {
      if (sticky.current.inline.has(b)) continue;
      const owned = ordered.filter((c) => first.get(c.n) === b);
      if (!owned.length) continue;
      const slots = owned.map((c) => {
        const marker = t.querySelector<HTMLElement>(`[data-cite="${c.n}"]`);
        const note = noteEls.current.get(c.n);
        return marker && note ? { anchor: Math.max(0, marker.getBoundingClientRect().top - base - NUDGE), height: note.offsetHeight } : null;
      });
      if (slots.some((s) => !s)) continue;
      // Try this paragraph's notes in the margin, below whatever is already there.
      let f2 = floor;
      const placed = placeNotes(
        slots.map((s) => ({ anchor: Math.max(s!.anchor, f2), height: s!.height })),
        GAP,
      );
      const push = Math.max(...placed.map((p, i) => p - slots[i]!.anchor));
      if (push > PUSH_LIMIT) {
        sticky.current.inline.add(b); // the text below will move; the resize re-measures
        setLayout({ compact: false, inline: new Set(sticky.current.inline), tops: {}, height: 0 });
        return;
      }
      owned.forEach((c, i) => {
        tops[c.n] = placed[i];
        bottom = Math.max(bottom, placed[i] + slots[i]!.height);
        f2 = placed[i] + slots[i]!.height + GAP;
      });
      floor = f2;
    }
    setLayout({ compact: false, inline: new Set(sticky.current.inline), tops, height: bottom });
  }, [answer, citations, blocks, ordered, first]);

  useLayoutEffect(() => measure(), [measure]);
  // Anything that changes a size can move a marker or a note: the frame, the text
  // (reflow, e.g. when a web font arrives) and each note. Late fonts are the one that bites.
  useEffect(() => {
    const f = frame.current;
    const t = text.current;
    if (!f || !t) return;
    const ro = new ResizeObserver(() => measure());
    ro.observe(f);
    ro.observe(t);
    noteEls.current.forEach((el) => ro.observe(el));
    const onFonts = () => measure();
    document.fonts?.addEventListener("loadingdone", onFonts);
    void document.fonts?.ready.then(onFonts);
    return () => {
      ro.disconnect();
      document.fonts?.removeEventListener("loadingdone", onFonts);
    };
  }, [measure, layout.compact]);

  const pin = useCallback(
    (n: number) => {
      setPinned((p) => (p === n ? null : n));
      // Reading flow: a marker takes you to its note, and Enter there opens the document.
      const target = frame.current?.querySelector<HTMLElement>(`[data-open="${n}"]`);
      const visible = [...(frame.current?.querySelectorAll<HTMLElement>(`[data-open="${n}"]`) ?? [])].find((el) => el.getClientRects().length > 0);
      (visible ?? target)?.focus({ preventScroll: false });
    },
    [],
  );
  const link: Link = { active: hover ?? pinned, setActive: setHover, pin, known };

  const inlineAt = (b: number) => layout.compact || layout.inline.has(b);
  const cls = "answer-frame" + (layout.compact ? " is-compact" : "") + (animate ? " is-animated" : "") + (fill ? " is-filling" : "");

  return (
    <div ref={frame} className={cls} data-compact={layout.compact} data-inline-blocks={[...layout.inline].join(",")}>
      <div ref={text} className="answer-text">
        {slot && layout.compact && <div className="slot-above">{slot}</div>}
        {blocks.map((b, i) => (
          <Fragment key={i}>
            <BlockView block={b} link={link} />
            {inlineAt(i) &&
              ordered
                .filter((c) => first.get(c.n) === i)
                .map((c, k) => <Note key={c.n} c={c} index={k} link={link} variant="inline" onOpen={onOpen} />)}
          </Fragment>
        ))}
      </div>
      {!layout.compact && (
        <div className="margin" style={{ height: layout.height }}>
          {slot && !ordered.length && <div className="slot-above">{slot}</div>}
          {slotFills && ordered.length > 0 && (
            <Slot teams={fill.teams} variant="margin" completing style={{ top: layout.tops[ordered[0].n] ?? 0 }} onDone={() => setSlotDone(true)} />
          )}
          {ordered.map((c, k) => {
            const inline = layout.inline.has(first.get(c.n) ?? -1);
            return (
              <Note
                key={c.n}
                c={c}
                index={k}
                link={link}
                variant={inline ? "shadow" : "margin"}
                style={{ top: layout.tops[c.n] ?? 0 }}
                onOpen={onOpen}
                refCb={(el) => {
                  if (el) noteEls.current.set(c.n, el);
                  else noteEls.current.delete(c.n);
                }}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}
