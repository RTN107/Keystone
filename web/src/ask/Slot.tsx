import { Stone } from "../ui/Mark";

// "Sales & Marketing", "A and B", "A, B and C". Teams are named; what they hold never is.
export function listTeams(teams: string[]): string {
  if (teams.length === 0) return "another team";
  if (teams.length === 1) return teams[0];
  return `${teams.slice(0, -1).join(", ")} and ${teams[teams.length - 1]}`;
}

// The sources margin of a blocked answer: the same geometry as a note (a rule on the left,
// a head, a caption), holding an empty keystone-shaped slot where a source would sit.
// It repeats what the message already says, so it is hidden from assistive technology.
export function Slot({ teams, variant, completing = false, style, onDone }: {
  teams: string[];
  variant: "margin" | "inline";
  // True while the answer that fills it arrives: the stone fills, then the slot gives way to the note.
  completing?: boolean;
  style?: React.CSSProperties;
  onDone?: () => void;
}) {
  return (
    <aside className={`slot is-${variant}` + (completing ? " is-completing" : "")} style={style} aria-hidden="true" onAnimationEnd={(e) => e.target === e.currentTarget && onDone?.()}>
      <span className="slot-stone">
        <Stone />
      </span>
      <span className="slot-text">
        <span className="slot-held">Held by {listTeams(teams)}</span>
        <span className="slot-sub">Not open to your team</span>
      </span>
    </aside>
  );
}
