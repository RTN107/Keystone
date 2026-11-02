// Turns a document's stored text into blocks for display while keeping every character's
// position. The server's offsets are UTF-16 code units into this exact string, and a JS
// string index is a UTF-16 code unit, so a range can be compared to a line's start directly.

export type Line = { text: string; start: number; end: number };
export type DocBlock =
  | { kind: "h1" | "h2"; line: Line }
  | { kind: "text"; lines: Line[] }
  | { kind: "list"; ordered: boolean; lines: Line[] }
  | { kind: "table"; rows: Line[] };

export function lineOffsets(content: string): Line[] {
  const out: Line[] = [];
  let pos = 0;
  for (;;) {
    const nl = content.indexOf("\n", pos);
    const end = nl < 0 ? content.length : nl;
    out.push({ text: content.slice(pos, end), start: pos, end });
    if (nl < 0) break;
    pos = nl + 1;
  }
  return out;
}

const isRow = (t: string) => t.startsWith("|");
const isSeparator = (t: string) => /^\|[\s|:-]+\|?$/.test(t) && t.includes("-");
const isItem = (t: string) => /^(\d+\.|[-*])\s+/.test(t);

export function parseDocument(content: string): DocBlock[] {
  const lines = lineOffsets(content);
  const blocks: DocBlock[] = [];
  let i = 0;
  while (i < lines.length) {
    const l = lines[i];
    if (l.text.trim() === "") { i++; continue; }
    if (l.text.startsWith("## ")) { blocks.push({ kind: "h2", line: l }); i++; continue; }
    if (l.text.startsWith("# ")) { blocks.push({ kind: "h1", line: l }); i++; continue; }
    const group: Line[] = [];
    if (isRow(l.text)) {
      while (i < lines.length && isRow(lines[i].text)) group.push(lines[i++]);
      blocks.push({ kind: "table", rows: group });
    } else if (isItem(l.text)) {
      while (i < lines.length && isItem(lines[i].text)) group.push(lines[i++]);
      blocks.push({ kind: "list", ordered: /^\d/.test(group[0].text), lines: group });
    } else {
      while (i < lines.length && lines[i].text.trim() !== "" && !isRow(lines[i].text) && !isItem(lines[i].text) && !lines[i].text.startsWith("#")) group.push(lines[i++]);
      blocks.push({ kind: "text", lines: group });
    }
  }
  return blocks;
}

export const cells = (row: string) => row.replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
export const tableRows = (rows: Line[]) => rows.filter((r) => !isSeparator(r.text));

// True if the block's characters overlap [start, end).
export function overlaps(b: DocBlock, start: number, end: number): boolean {
  const ls: Line[] = b.kind === "h1" || b.kind === "h2" ? [b.line] : b.kind === "table" ? b.rows : (b as { lines: Line[] }).lines;
  return ls.some((l) => l.start < end && l.end > start);
}

// A passage's text as a plain excerpt: a table (stored as pipe-delimited lines) reads as
// "Priority · Acknowledge · Mitigate within" instead of showing the markdown. The separator row
// is dropped. Only the margin excerpt uses this; the source viewer renders a real table.
export function plainExcerpt(text: string): string {
  return text
    .split("\n")
    .filter((l) => !isSeparator(l.trim()))
    .map((l) => (isRow(l.trim()) ? cells(l.trim()).join(" \u00b7 ") : l))
    .join("\n");
}
