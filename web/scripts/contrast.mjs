// WCAG 2.x contrast ratios for every colour pair the interface ships. Run:
//   node scripts/contrast.mjs
// Exits non-zero if any pair is below its target, so it can gate a build.
const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
const lin = (c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const lum = (h) => { const [r, g, b] = hex(h).map(lin); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
const ratio = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
const mix = (fg, bg, pct) => "#" + hex(fg).map((c, i) => Math.round((c * pct + hex(bg)[i] * (1 - pct)) * 255).toString(16).padStart(2, "0")).join("");

const c = { wash: "#edf0f2", sheet: "#fbfcfc", ink: "#17263a", muted: "#52606f", signal: "#00f0ff", ochre: "#8a5100", rule: "#c9d1d8", field: "#7a8794", inkHover: "#24374f" };
// Electric blue is only ever a fill, so it appears here only as the surface something else sits on.
const chipRest = mix(c.signal, c.wash, 0.25); // a marker or note number at rest, on the page
const chipOn = mix(c.signal, c.wash, 0.4); // a marker hovered or active, and a note number of the active note
const band = mix(c.signal, c.sheet, 0.4); // the cited passage in the viewer
const socket = mix(c.ink, c.wash, 0.09); // the empty slot's tint
const rowLanded = mix(c.signal, c.wash, 0.25); // a document row that has just been added

// Each pair is checked where it is actually placed: the answer and margin sit on the slate wash,
// the viewer and the waiting card sit on the sheet.
const pairs = [
  // [name, foreground, background, target]
  ["answer text and question, ink on wash", c.ink, c.wash, 4.5],
  ["hint, notes text, muted ink on wash", c.muted, c.wash, 4.5],
  ["marker and note number, ink numeral on the resting chip", c.ink, chipRest, 4.5],
  ["marker and note number, ink numeral on the hover and active chip", c.ink, chipOn, 4.5],
  ["admin document row that has just landed, ink on its tint", c.ink, rowLanded, 4.5],
  ["header and cards, ink on sheet", c.ink, c.sheet, 4.5],
  ["viewer subtitle, muted ink on sheet", c.muted, c.sheet, 4.5],
  ["viewer text inside the cited band, ink on the 40% blue", c.ink, band, 4.5],
  ["viewer subtitle inside the cited band, muted on the 40% blue", c.muted, band, 4.5],
  ["primary button label, sheet on ink", c.sheet, c.ink, 4.5],
  ["primary button label on hover, sheet on hover ink", c.sheet, c.inkHover, 4.5],
  ["quiet button and link label, ink on sheet", c.ink, c.sheet, 4.5],
  ["quiet link, muted ink on sheet (header)", c.muted, c.sheet, 4.5],
  ["focus ring, ink vs wash", c.ink, c.wash, 3],
  ["focus ring, ink vs sheet", c.ink, c.sheet, 3],
  ["focus ring on the cited band, ink vs the 40% blue", c.ink, band, 3],
  ["active note border, ink vs wash", c.ink, c.wash, 3],
  ["cited band rule, ink vs sheet", c.ink, c.sheet, 3],
  ["cited band rule against the band it points at, ink vs the 40% blue", c.ink, band, 3],
  ["error rule, ochre vs wash", c.ochre, c.wash, 3],
  ["query log outcome text, Blocked in ochre on wash", c.ochre, c.wash, 4.5],
  ["extended tag and result rule edge, ink vs sheet (non-text)", c.ink, c.sheet, 3],
  ["selected outcome filter, sheet on ink", c.sheet, c.ink, 4.5],
  ["waiting bar fill vs its track (non-text)", c.muted, c.wash, 3],
  ["field edge (composer, inputs, quiet buttons) vs wash", c.field, c.wash, 3],
  ["field edge vs sheet", c.field, c.sheet, 3],
  ["resting note border, rule vs wash (decorative)", c.rule, c.wash, 1.2],
  ["empty slot edge, muted ink vs wash (non-text)", c.muted, c.wash, 3],
  ["empty slot edge, muted ink vs its own tint", c.muted, socket, 3],
  ["slot caption, ink on wash", c.ink, c.wash, 4.5],
  ["slot sub-caption, muted ink on wash", c.muted, c.wash, 4.5],
  ["filled slot edge, ink vs wash (non-text)", c.ink, c.wash, 3],
  ["filled slot edge against its own full-strength fill, ink vs signal", c.ink, c.signal, 3],
  ["blocked message and verdict, ink on wash", c.ink, c.wash, 4.5],
  ["checked-again line, muted ink on wash", c.muted, c.wash, 4.5],
  // Informational: these say how faint the blue is as a fill. They are NOT requirements. A fill carries no
  // meaning by itself here; the ink next to it does (checked above).
  ["INFO the 25% chip against the wash (it has no perceivable edge)", chipRest, c.wash, 1],
  ["INFO the cited band against the sheet", band, c.sheet, 1],
  ["INFO full-strength signal against the wash (the slot filling)", c.signal, c.wash, 1],
  ["INFO full-strength signal against the sheet", c.signal, c.sheet, 1],
];
let bad = 0;
for (const [name, fg, bg, target] of pairs) {
  const r = ratio(fg, bg);
  const ok = r >= target;
  if (!ok) bad++;
  console.log(`${ok ? "pass" : "FAIL"}  ${r.toFixed(2).padStart(5)}:1  (need ${target})  ${name}  ${fg} on ${bg}`);
}

// Colour distance (CIE76 in Lab, D65), for the question contrast ratio cannot answer: is ochre, the blocked
// state, still its own colour now that the accent is cool? Ochre must sit far from ink and from the signal.
const lab = (h) => {
  const [r, g, b] = hex(h).map(lin);
  const xyz = [(0.4124 * r + 0.3576 * g + 0.1805 * b) / 0.95047, (0.2126 * r + 0.7152 * g + 0.0722 * b), (0.0193 * r + 0.1192 * g + 0.9505 * b) / 1.08883];
  const f = (t) => (t > 0.008856 ? Math.cbrt(t) : 7.787 * t + 16 / 116);
  const [fx, fy, fz] = xyz.map(f);
  return [116 * fy - 16, 500 * (fx - fy), 200 * (fy - fz)];
};
const dE = (a, b) => Math.hypot(...lab(a).map((v, i) => v - lab(b)[i]));
const hue = (h) => { const [, a, b] = lab(h); return ((Math.atan2(b, a) * 180) / Math.PI + 360) % 360; };
for (const [name, a, b, min] of [["ochre vs ink", c.ochre, c.ink, 35], ["ochre vs muted ink", c.ochre, c.muted, 35], ["ochre vs the 25% chip", c.ochre, chipRest, 35], ["ochre vs full signal", c.ochre, c.signal, 35]]) {
  const d = dE(a, b);
  const ok = d >= min;
  if (!ok) bad++;
  console.log(`${ok ? "pass" : "FAIL"}  dE ${d.toFixed(1).padStart(5)}  (need ${min})  ${name}  (hue ${hue(a).toFixed(0)} vs ${hue(b).toFixed(0)} degrees)`);
}
// The same question under colour-vision deficiency (Machado, Oliveira and Fernandes 2009, severity 1.0, in linear
// RGB). Ochre carries the blocked state; it must stay its own colour for someone who does not see hue as most do.
const MATRIX = {
  protanopia: [[0.152286, 1.052583, -0.204868], [0.114503, 0.786281, 0.099216], [-0.003882, -0.048116, 1.051998]],
  deuteranopia: [[0.367322, 0.860646, -0.227968], [0.280085, 0.672501, 0.047413], [-0.01182, 0.04294, 0.968881]],
  tritanopia: [[1.255528, -0.076749, -0.178779], [-0.078411, 0.930809, 0.147602], [0.004733, 0.691367, 0.3039]],
};
const simulate = (h, m) => { const l = hex(h).map(lin); return MATRIX[m].map((row) => Math.min(1, Math.max(0, row[0] * l[0] + row[1] * l[1] + row[2] * l[2]))); };
const labRgb = ([r, g, b]) => { const xyz = [(0.4124 * r + 0.3576 * g + 0.1805 * b) / 0.95047, 0.2126 * r + 0.7152 * g + 0.0722 * b, (0.0193 * r + 0.1192 * g + 0.9505 * b) / 1.08883]; const f = (t) => (t > 0.008856 ? Math.cbrt(t) : 7.787 * t + 16 / 116); const [fx, fy, fz] = xyz.map(f); return [116 * fy - 16, 500 * (fx - fy), 200 * (fy - fz)]; };
for (const m of Object.keys(MATRIX)) for (const [name, ref] of [["ink", c.ink], ["muted ink", c.muted], ["the 25% chip", chipRest]]) {
  const a = labRgb(simulate(c.ochre, m)), b = labRgb(simulate(ref, m));
  const d = Math.hypot(...a.map((v, i) => v - b[i]));
  const ok = d >= 40;
  if (!ok) bad++;
  console.log(`${ok ? "pass" : "FAIL"}  dE ${d.toFixed(1).padStart(5)}  (need 40)  ochre vs ${name} as seen with ${m}`);
}
process.exit(bad ? 1 : 0);
