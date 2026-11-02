// Where each margin note goes. Every note wants to sit level with the first place
// its source is cited. When two notes want the same space, the later one moves
// down just far enough to clear the earlier one. Pure, so it can be tested alone.

export type NoteSlot = { anchor: number; height: number };

export function placeNotes(slots: NoteSlot[], gap: number): number[] {
  const order = slots.map((_, i) => i).sort((a, b) => slots[a].anchor - slots[b].anchor || a - b);
  const tops = new Array<number>(slots.length).fill(0);
  let floor = Number.NEGATIVE_INFINITY;
  for (const i of order) {
    const top = Math.max(slots[i].anchor, floor);
    tops[i] = top;
    floor = top + slots[i].height + gap;
  }
  return tops;
}
