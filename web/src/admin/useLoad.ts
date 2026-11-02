import { useCallback, useEffect, useState } from "react";

// Load something, say so while it loads, say what went wrong if it fails, and let the caller reload it.
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [state, setState] = useState<{ data: T | null; error: string | null; loading: boolean }>({ data: null, error: null, loading: true });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const run = useCallback(fn, deps);
  const reload = useCallback(() => {
    setState((s) => ({ ...s, loading: true, error: null }));
    run().then(
      (data) => setState({ data, error: null, loading: false }),
      (e) => setState((s) => ({ data: s.data, error: e instanceof Error ? e.message : "Something went wrong.", loading: false })),
    );
  }, [run]);
  useEffect(() => reload(), [reload]);
  return { ...state, reload };
}

// "3 Nov, 14:05" in the viewer's own time zone, 24 hour, so a log reads the same way everywhere.
export const when = (iso: string) => new Date(iso).toLocaleString("en-IN", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit", hour12: false });
export const whenLong = (iso: string) => new Date(iso).toLocaleString("en-IN", { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit", hour12: false });
