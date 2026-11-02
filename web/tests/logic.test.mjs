// The two pieces of the Ask screen that are plain logic: where margin notes go, and what the
// person is looking at as the stream arrives. Run with `npm test` (Node 22 reads the TypeScript directly).
import test from "node:test";
import assert from "node:assert/strict";
import { placeNotes } from "../src/ask/layout.ts";
import { reduce, sameQuestion } from "../src/ask/state.ts";

test("notes that do not collide stay level with their markers", () => {
  assert.deepEqual(placeNotes([{ anchor: 10, height: 50 }, { anchor: 200, height: 50 }], 10), [10, 200]);
});

test("a note that would overlap the one above moves down just far enough, leaving the gap", () => {
  assert.deepEqual(placeNotes([{ anchor: 10, height: 50 }, { anchor: 30, height: 50 }], 10), [10, 70]);
});

test("the result follows the order of the input, not the order of the anchors", () => {
  assert.deepEqual(placeNotes([{ anchor: 100, height: 40 }, { anchor: 0, height: 40 }], 8), [100, 0]);
  assert.deepEqual(placeNotes([{ anchor: 0, height: 60 }, { anchor: 0, height: 60 }, { anchor: 0, height: 60 }], 10), [0, 70, 140]);
});

test("no notes, no positions", () => {
  assert.deepEqual(placeNotes([], 10), []);
});

const ev = (e) => ({ type: "event", ev: e });
const done = (d) => ev({ type: "done", done: { citations: [], ...d } });

test("an answered question goes searching, writing, answered", () => {
  let v = reduce({ phase: "idle" }, { type: "ask", question: "Q" }, 0);
  assert.equal(v.phase, "searching");
  v = reduce(v, ev({ type: "start", outcome: "answered" }), 1);
  assert.equal(v.phase, "writing");
  v = reduce(v, ev({ type: "token", text: "Hello " }), 2);
  v = reduce(v, ev({ type: "token", text: "there [1]." }), 3);
  assert.equal(v.text, "Hello there [1].");
  v = reduce(v, done({ outcome: "answered", answer: "Hello there [1].", citations: [{ n: 1 }] }), 4);
  assert.equal(v.phase, "answered");
  assert.equal(v.citations.length, 1);
});

test("a blocked question never passes through writing and carries the holding teams", () => {
  let v = reduce({ phase: "idle" }, { type: "ask", question: "Q" }, 0);
  v = reduce(v, ev({ type: "start", outcome: "blocked" }), 1);
  assert.equal(v.phase, "searching");
  v = reduce(v, done({ outcome: "blocked", message: "m", holding_teams: ["Sales & Marketing"], your_team: "Delivery" }), 2);
  assert.deepEqual([v.phase, v.holdingTeams, v.yourTeam], ["blocked", ["Sales & Marketing"], "Delivery"]);
});

test("waiting keeps the time it first started, and the next attempt replaces the message", () => {
  let v = reduce({ phase: "searching", question: "Q" }, ev({ type: "waiting", attempt: 1, maxAttempts: 5, retryIn: 20, message: "a" }), 100);
  const first = v.firstAt;
  v = reduce(v, ev({ type: "waiting", attempt: 2, maxAttempts: 5, retryIn: 20, message: "b" }), 20100);
  assert.equal(v.firstAt, first);
  assert.equal(v.message, "b");
});

test("a failure keeps the question and the partial text that was streaming", () => {
  const v = reduce({ phase: "writing", question: "Q", text: "partial", startedAt: 0 }, { type: "fail", code: "cut_off", message: "x" }, 0);
  assert.deepEqual([v.phase, v.question, v.partialText], ["error", "Q", "partial"]);
});

test("the same question means the same words, ignoring case and spacing", () => {
  assert.equal(sameQuestion("What discount can I offer?", "  what   discount can I OFFER? "), true);
  assert.equal(sameQuestion("What discount can I offer?", "What discount can we offer?"), false);
});

import { parse, hrefFor } from "../src/admin/route.ts";

test("the admin hash router reads each view, and anything unknown is the documents list", () => {
  assert.deepEqual(parse(""), { view: "documents" });
  assert.deepEqual(parse("#/documents"), { view: "documents" });
  assert.deepEqual(parse("#/ingestion"), { view: "ingestion" });
  assert.deepEqual(parse("#/queries"), { view: "queries" });
  assert.deepEqual(parse("#/people"), { view: "people" });
  assert.deepEqual(parse("#/nonsense"), { view: "documents" });
});

test("the access screen needs a numeric document id and round-trips through its link", () => {
  assert.deepEqual(parse("#/documents/12/access"), { view: "access", id: 12 });
  assert.deepEqual(parse("#/documents/abc/access"), { view: "documents" });
  assert.deepEqual(parse("#/documents/12"), { view: "documents" });
  assert.equal(hrefFor({ view: "access", id: 12 }), "#/documents/12/access");
  assert.deepEqual(parse(hrefFor({ view: "access", id: 7 })), { view: "access", id: 7 });
});

import { readFileSync } from "node:fs";

// Vite bundles every stylesheet into one, so a rule in admin.css reaches the Ask screen unless it is scoped.
// Once, `.result` and `.block` in admin.css restyled the answer text (a green rule, grey words, big gaps).
test("every selector in admin.css is scoped under .admin, so it cannot restyle another screen", () => {
  const css = readFileSync(new URL("../src/admin/admin.css", import.meta.url), "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  const unscoped = [];
  let inKeyframes = 0;
  for (const m of css.matchAll(/([^{}]+)\{/g)) {
    const sel = m[1].trim();
    if (sel.startsWith("@keyframes")) { inKeyframes = 1; continue; }
    if (sel.startsWith("@")) continue;
    if (inKeyframes && /^(from|to|\d+%)(\s*,\s*(from|to|\d+%))*$/.test(sel)) continue;
    inKeyframes = 0;
    for (const part of sel.split(",").map((s) => s.trim())) if (!/^\.admin([\s:.[]|$)/.test(part)) unscoped.push(part);
  }
  assert.deepEqual(unscoped, [], `unscoped admin selectors: ${unscoped.join(" | ")}`);
});

// Chrome reads the channels of a color-mix() start as Oklab coordinates when a keyframe animates it to a hex
// colour, and the filling slot flashed yellow instead of electric blue. Nothing the slot animates may be a color-mix().
test("the slot's stone is drawn and animated in plain colours, never a color-mix()", () => {
  const css = readFileSync(new URL("../src/ask/ask-screen.css", import.meta.url), "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  const blocks = [...css.matchAll(/(\.stone[\w-]*|@keyframes stone-[\w-]+)[^{]*\{((?:[^{}]|\{[^{}]*\})*)\}/g)];
  assert.ok(blocks.length >= 5, "expected the .stone rules and the stone keyframes");
  for (const [, name, body] of blocks) assert.ok(!/color-mix/.test(body), `${name} must not use color-mix()`);
});

import { MARK_BOX, SMALL, FULL, bodyPath, sliverPath, markSvg, faviconSvg } from "../src/ui/stoneGeometry.ts";

test("the icon files on disk are exactly what the one geometry module generates", () => {
  assert.equal(readFileSync(new URL("../public/mark.svg", import.meta.url), "utf8").trim(), markSvg());
  assert.equal(readFileSync(new URL("../public/favicon.svg", import.meta.url), "utf8").trim(), faviconSvg());
});

test("the seated stone is one wedge cut once: the body ends above the sliver, with a gap between", () => {
  for (const s of [FULL, SMALL]) {
    const body = /H[\d.]+/.exec(bodyPath(s)); // the body's flat bottom edge
    const yb = Number(/L[\d.]+ ([\d.]+)H/.exec(bodyPath(s))[1]);
    const yt = Number(/^M[\d.]+ ([\d.]+)H/.exec(sliverPath(s))[1]);
    assert.ok(body && yt - yb > 0, "the sliver starts below where the body ends");
    assert.ok(Math.abs(yt - yb - s.gap) < 0.02, "the gap between the cut edges is the shape's gap");
  }
  assert.ok(SMALL.gap < FULL.gap, "the 16px variant has the narrower gap");
  assert.ok(SMALL.topR - SMALL.topL > FULL.topR - FULL.topL, "the 16px variant is wider at the top");
  assert.equal(MARK_BOX, "6 5.4 20 22");
});

// The viewer marks a cited passage by comparing the server's character range with each parsed
// block (heading, paragraph, list, table). A table is one block, so the marking is by whole table.
// The chunker makes each "##" section of the demo documents exactly one passage (the ingest tests pin
// the passage counts to the section counts), so a section's own range is a faithful citation.
import { readdirSync } from "node:fs";
import { parseDocument, overlaps, plainExcerpt } from "../src/ask/docText.ts";

const docsDir = new URL("../../demo-docs/", import.meta.url);
const sectionsOf = (content) => {
  const starts = [...content.matchAll(/^## /gm)].map((m) => m.index);
  return starts.map((s, i) => ({ start: s, end: (starts[i + 1] ?? content.length) }));
};

test("every section of every demo document marks exactly its own blocks, tables included", () => {
  let tables = 0;
  for (const f of readdirSync(docsDir).filter((n) => n.endsWith(".md"))) {
    const content = readFileSync(new URL(f, docsDir), "utf8");
    const blocks = parseDocument(content);
    const secs = sectionsOf(content);
    for (const sec of secs) {
      const range = { start: sec.start, end: sec.end - 1 }; // the passage ends before the blank line that follows it
      const marked = blocks.filter((b) => overlaps(b, range.start, range.end));
      const inside = blocks.filter((b) => {
        const first = b.kind === "h1" || b.kind === "h2" ? b.line : b.kind === "table" ? b.rows[0] : b.lines[0];
        return first.start >= sec.start && first.start < sec.end;
      });
      assert.deepEqual(marked, inside, `${f}: section at ${sec.start}`);
      tables += marked.filter((b) => b.kind === "table").length;
    }
  }
  assert.ok(tables >= 4, "the demo documents have tables, so this test must reach some");
});

test("a range inside one table row marks that whole table and nothing around it (over-marks, never under-marks)", () => {
  const content = readFileSync(new URL("delivery-and-escalation-sop.md", docsDir), "utf8");
  const blocks = parseDocument(content);
  const table = blocks.find((b) => b.kind === "table");
  const row = table.rows[2];
  const marked = blocks.filter((b) => overlaps(b, row.start + 1, row.start + 5));
  assert.deepEqual(marked, [table]);
});

test("a range ends exclusive: ending exactly where a block begins does not mark that block", () => {
  const content = readFileSync(new URL("delivery-and-escalation-sop.md", docsDir), "utf8");
  const blocks = parseDocument(content);
  const table = blocks.find((b) => b.kind === "table");
  assert.equal(blocks.filter((b) => overlaps(b, 0, table.rows[0].start) && b.kind === "table").length, 0);
});

test("a table passage reads as plain text in the margin excerpt: no pipes, no separator row", () => {
  const raw = "| Priority | Acknowledge | Update the client | Mitigate within |\n|---|---|---|---|\n| P1 | 15 minutes, any hour | every 30 minutes | 4 hours |";
  const out = plainExcerpt(raw);
  assert.ok(!out.includes("|") && !out.includes("---"), out);
  assert.equal(out.split("\n").length, 2);
  assert.ok(out.startsWith("Priority \u00b7 Acknowledge") && out.includes("P1 \u00b7 15 minutes, any hour"), out);
  assert.equal(plainExcerpt("Ordinary text with a | in the middle stays as it is."), "Ordinary text with a | in the middle stays as it is.");
});
