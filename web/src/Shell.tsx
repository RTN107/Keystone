import type { ReactNode } from "react";
import type { Employee, Health } from "./api";
import { Mark } from "./ui/Mark";
import "./shell.css";

export function Shell({ me, health, onSignOut, children }: { me: Employee | null; health: Health | null; onSignOut: () => void; children: ReactNode }) {
  return (
    <>
      <a className="skip" href="#main">
        Skip to the main content
      </a>
      {me && (
        <header className="bar">
          <div className="bar-inner">
            <span className="brand">
              <Mark size={28} />
              <span className="brand-name">Keystone</span>
            </span>
            <span className="bar-who">
              <span className="who-name">{me.name}</span>
              {(() => {
                const label = me.role === "admin" ? "Admin" : me.team?.name;
                return label && label !== me.name ? <span className="who-team">{label}</span> : null;
              })()}
              <button type="button" className="link-btn" onClick={onSignOut}>
                Sign out
              </button>
            </span>
          </div>
        </header>
      )}
      {me && health?.read_only && (
        <p className="strip">This deployment is read-only. You can ask questions, but documents, access and accounts cannot be changed here.</p>
      )}
      {children}
    </>
  );
}
