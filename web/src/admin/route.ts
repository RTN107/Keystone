import { useEffect, useState } from "react";

// Five views, one hash each. A router library would be more than this needs.
export type Route =
  | { view: "documents" }
  | { view: "access"; id: number }
  | { view: "ingestion" }
  | { view: "queries" }
  | { view: "people" };

export function parse(hash: string): Route {
  const h = hash.replace(/^#\/?/, "");
  const a = /^documents\/(\d+)\/access$/.exec(h);
  if (a) return { view: "access", id: Number(a[1]) };
  if (h === "ingestion") return { view: "ingestion" };
  if (h === "queries") return { view: "queries" };
  if (h === "people") return { view: "people" };
  return { view: "documents" };
}

export const hrefFor = (r: Route) => (r.view === "access" ? `#/documents/${r.id}/access` : `#/${r.view === "documents" ? "documents" : r.view}`);

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parse(location.hash));
  useEffect(() => {
    const on = () => setRoute(parse(location.hash));
    addEventListener("hashchange", on);
    return () => removeEventListener("hashchange", on);
  }, []);
  return route;
}
