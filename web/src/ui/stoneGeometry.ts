// The seated stone: ONE wedge, ONE horizontal cut. This is the single source of the drawing. The mark, the empty
// slot, the favicon and the generated SVG files all come from here, so they cannot drift apart.
//
// The wedge keeps the shape that makes it read as a ring segment and not as a cup: a convex extrados above and
// a concave intrados below. A thin base sliver is cut off near the bottom, with a transparent gap between it and
// the body. Seated: the gap is small and the sliver is lit. Unseated (the empty slot in the sources margin): the
// sliver sits lower, in --field. Seating the stone is one translate of the sliver plus one colour change.

export type StoneShape = {
  topL: number; topR: number; topY: number; topCtrl: number; // extrados: a quadratic through (16, topCtrl)
  botL: number; botR: number; botY: number; botCtrl: number; // intrados: a quadratic through (16, botCtrl)
  cutY: number; // the horizontal cut
  gap: number; // between the two cut edges, before the stroke eats into it
  stroke: number; // a round-joined stroke in the fill colour gives the corners a radius of stroke / 2
};

// The full mark, drawn for 28px and up.
export const FULL: StoneShape = { topL: 7, topR: 25, topY: 8.2, topCtrl: 4.6, botL: 10.8, botR: 21.2, botY: 26.4, botCtrl: 24.4, cutY: 22, gap: 3, stroke: 1.6 };

// The 16px optical size, used only for the favicon. Same drawing, adjusted for 16 pixels: the extrados bows more
// so the ring segment survives, the top is wider against the base so the taper is unmistakable, and the gap is
// narrower so the seam does not eat a whole pixel row. The intrados is softened for the same reason: at 16px the
// sliver is two pixels thick and a deep concave underside splits into two feet, like a tooth. It is the same mark
// seen smaller, not a second drawing.
export const SMALL: StoneShape = { topL: 5.6, topR: 26.4, topY: 9, topCtrl: 2.9, botL: 11.4, botR: 20.6, botY: 26.6, botCtrl: 25.4, cutY: 22.3, gap: 1.8, stroke: 0.9 };

// How far the sliver sits below its seated place when the stone is unseated, in the FULL drawing's units.
export const DROP = 2.4;

const n = (v: number) => String(Math.round(v * 100) / 100);
const slope = (s: StoneShape) => (s.botL - s.topL) / (s.botY - s.topY);
const xl = (s: StoneShape, y: number) => s.topL + (y - s.topY) * slope(s);
const xr = (s: StoneShape, y: number) => s.topR - (y - s.topY) * slope(s);

export function bodyPath(s: StoneShape = FULL): string {
  const yb = s.cutY - s.gap / 2;
  return `M${n(s.topL)} ${n(s.topY)}Q16 ${n(s.topCtrl)} ${n(s.topR)} ${n(s.topY)}L${n(xr(s, yb))} ${n(yb)}H${n(xl(s, yb))}Z`;
}

export function sliverPath(s: StoneShape = FULL, drop = 0): string {
  const yt = s.cutY + s.gap / 2;
  return `M${n(xl(s, yt))} ${n(yt + drop)}H${n(xr(s, yt))}L${n(s.botR)} ${n(s.botY + drop)}Q16 ${n(s.botCtrl + drop)} ${n(s.botL)} ${n(s.botY + drop)}Z`;
}

// The viewBox of the FULL mark: the stone's exact bounds, stroke included. The unseated stone is taller by DROP.
export const MARK_BOX = "6 5.4 20 22";
export const SLOT_BOX = `6 5.4 20 ${22 + DROP}`;

// A square box centred on a shape, with a little air, for the favicon.
export function squareBox(s: StoneShape, air = 0.6): string {
  const top = (s.topY + s.topCtrl) / 2 - s.stroke / 2;
  const bottom = s.botY + s.stroke / 2;
  const side = Math.max(bottom - top, Math.max(s.topR, s.botR) - Math.min(s.topL, s.botL) + s.stroke) + 2 * air;
  const cx = 16, cy = (top + bottom) / 2;
  return `${n(cx - side / 2)} ${n(cy - side / 2)} ${n(side)} ${n(side)}`;
}

export const COLORS = { ink: "#17263a", signal: "#00f0ff", field: "#7a8794", sheet: "#fbfcfc", wash: "#edf0f2" };

const strokeAttrs = (s: StoneShape) => `stroke-width="${s.stroke}" stroke-linejoin="round"`;

// mark.svg: the seated stone on its own, in ink and signal, for the README header and anywhere an image is needed.
export function markSvg(s: StoneShape = FULL): string {
  const { ink, signal } = COLORS;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${MARK_BOX}"><path d="${bodyPath(s)}" fill="${ink}" stroke="${ink}" ${strokeAttrs(s)}/><path d="${sliverPath(s)}" fill="${signal}" stroke="${signal}" ${strokeAttrs(s)}/></svg>`;
}

// favicon.svg: the 16px optical size, on a square. Under a dark colour scheme the ink body becomes sheet.
export function faviconSvg(s: StoneShape = SMALL): string {
  const { ink, signal, sheet } = COLORS;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${squareBox(s)}"><style>.b{fill:${ink};stroke:${ink}}.s{fill:${signal};stroke:${signal}}.b,.s{stroke-width:${s.stroke};stroke-linejoin:round}@media (prefers-color-scheme:dark){.b{fill:${sheet};stroke:${sheet}}}</style><path class="b" d="${bodyPath(s)}"/><path class="s" d="${sliverPath(s)}"/></svg>`;
}
