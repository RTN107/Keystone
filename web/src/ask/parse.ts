// Turns the model's plain text into blocks the margin layout can measure. The
// answer is prose in short paragraphs, sometimes with a numbered or bulleted list,
// and [n] markers (already checked by the server against the supplied passages).

export type Inline = { kind: "text"; text: string } | { kind: "cite"; n: number };
export type Block =
  | { kind: "p"; inline: Inline[] }
  | { kind: "ol" | "ul"; items: Inline[][] };

const CITE = /\[(\d{1,3})\]/g;

export function inline(text: string): Inline[] {
  const out: Inline[] = [];
  let last = 0;
  for (const m of text.matchAll(CITE)) {
    // A marker hugs the word before it, like a footnote number: drop one space.
    if (m.index > last) out.push({ kind: "text", text: text.slice(last, m.index).replace(/ $/, "") });
    out.push({ kind: "cite", n: Number(m[1]) });
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push({ kind: "text", text: text.slice(last) });
  return out;
}

// Safe on a partial answer, so the same parser can run while the text streams.
export function parseAnswer(answer: string): Block[] {
  const blocks: Block[] = [];
  for (const chunk of answer.split(/\n{2,}/)) {
    let para: string[] = [];
    let list: { kind: "ol" | "ul"; items: string[] } | null = null;
    const flushPara = () => {
      if (para.length) blocks.push({ kind: "p", inline: inline(para.join(" ")) });
      para = [];
    };
    const flushList = () => {
      if (list) blocks.push({ kind: list.kind, items: list.items.map(inline) });
      list = null;
    };
    for (const line of chunk.split("\n")) {
      if (line.trim() === "") continue;
      const ol = /^\s*\d+\.\s+(.*)$/.exec(line);
      const ul = /^\s*[-*]\s+(.*)$/.exec(line);
      const item = ol ?? ul;
      if (item) {
        flushPara();
        const kind = ol ? "ol" : "ul";
        if (list && list.kind !== kind) flushList();
        (list ??= { kind, items: [] }).items.push(item[1]);
      } else {
        flushList();
        para.push(line.trim());
      }
    }
    flushPara();
    flushList();
  }
  return blocks;
}

// For each citation number, the index of the first block that cites it.
export function firstBlockOf(blocks: Block[]): Map<number, number> {
  const first = new Map<number, number>();
  blocks.forEach((b, i) => {
    const runs = b.kind === "p" ? [b.inline] : b.items;
    for (const run of runs) for (const part of run) if (part.kind === "cite" && !first.has(part.n)) first.set(part.n, i);
  });
  return first;
}
