import { DROP, MARK_BOX, SLOT_BOX, bodyPath, sliverPath } from "./stoneGeometry";

// The Keystone mark is the seated stone: an ink wedge with a thin base sliver cut off it, the seam lit in --signal.
// It is a keystone at the crown of an arch, the stone that locks the others in place, and it is seated.
// The drawing lives in stoneGeometry.ts and is shared with the empty slot below and with the icon files.
export function Mark({ size = 28, title }: { size?: number; title?: string }) {
  return (
    <svg className="keystone-mark" width={(size * 20) / 22} height={size} viewBox={MARK_BOX} role={title ? "img" : undefined} aria-label={title} aria-hidden={title ? undefined : true} focusable="false">
      <g strokeWidth="1.6" strokeLinejoin="round">
        <path d={bodyPath()} style={{ fill: "var(--ink)", stroke: "var(--ink)" }} />
        <path d={sliverPath()} style={{ fill: "var(--signal)", stroke: "var(--signal)" }} />
      </g>
    </svg>
  );
}

// The empty slot in the sources margin: the SAME drawing, unseated. The body is a quiet socket and the sliver sits
// lower, in --field. Seating it (ask-screen.css) hollows into ink, lifts the sliver into place and lights it:
// one element changing, not two drawings swapped.
export function Stone({ className = "" }: { className?: string }) {
  return (
    <svg className={`stone ${className}`.trim()} viewBox={SLOT_BOX} aria-hidden="true" focusable="false" data-drop={DROP}>
      <path className="stone-body" d={bodyPath()} />
      <path className="stone-sliver" d={sliverPath()} />
    </svg>
  );
}
