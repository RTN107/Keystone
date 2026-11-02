import { useEffect, useRef } from "react";
import type { Health } from "../api";
import { Documents } from "./Documents";
import { ExtendAccess } from "./ExtendAccess";
import { IngestionLog } from "./IngestionLog";
import { People } from "./People";
import { QueryLog } from "./QueryLog";
import { listTeams } from "./adminApi";
import { useRoute, type Route } from "./route";
import { useLoad } from "./useLoad";
import { useState } from "react";
import "./admin.css";

const NAV: { view: Route["view"]; href: string; label: string }[] = [
  { view: "documents", href: "#/documents", label: "Documents" },
  { view: "ingestion", href: "#/ingestion", label: "Ingestion log" },
  { view: "queries", href: "#/queries", label: "Query log" },
  { view: "people", href: "#/people", label: "People" },
];

export function AdminApp({ health }: { health: Health }) {
  const route = useRoute();
  const teams = useLoad(() => listTeams(), []);
  const [justAdded, setJustAdded] = useState<number | null>(null);
  const main = useRef<HTMLElement>(null);
  const first = useRef(true);
  const key = route.view === "access" ? `access-${route.id}` : route.view;

  // After the view changes, put the keyboard where the new page begins. Not on the first load.
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    main.current?.focus({ preventScroll: true });
    window.scrollTo({ top: 0 });
  }, [key]);

  const active = route.view === "access" ? "documents" : route.view;
  const readOnly = health.read_only;
  const embeddingsOn = health.capabilities.embeddings;

  return (
    <div className="admin">
      <nav className="admin-nav" aria-label="Admin">
        <ul>
          {NAV.map((n) => (
            <li key={n.view}>
              <a href={n.href} aria-current={active === n.view ? "page" : undefined}>
                {n.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
      <main id="main" ref={main} tabIndex={-1} aria-labelledby="view-title" className="admin-main">
        {teams.error && <p className="problem-line" role="alert">{teams.error}</p>}
        {!teams.data ? (
          !teams.error && <p className="quiet" role="status">Loading.</p>
        ) : route.view === "documents" ? (
          <Documents teams={teams.data} readOnly={readOnly} embeddingsOn={embeddingsOn} justAdded={justAdded} onAdded={setJustAdded} />
        ) : route.view === "access" ? (
          <ExtendAccess docId={route.id} teams={teams.data} readOnly={readOnly} />
        ) : route.view === "ingestion" ? (
          <IngestionLog embeddingsOn={embeddingsOn} />
        ) : route.view === "queries" ? (
          <QueryLog teams={teams.data} />
        ) : (
          <People teams={teams.data} readOnly={readOnly} onChanged={teams.reload} />
        )}
      </main>
    </div>
  );
}
