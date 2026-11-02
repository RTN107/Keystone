import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { api, ApiError, tokenStore, type Employee, type Health } from "./api";
import { AuthScreen } from "./auth/AuthScreen";
import { AskScreen } from "./ask/AskScreen";
import { Viewer } from "./ask/Viewer";
import type { Citation } from "./ask/AnswerWithMargin";
import { Shell } from "./Shell";
import { AdminApp } from "./admin/AdminApp";

const Lab = import.meta.env.DEV ? lazy(() => import("./ask/MarginLab").then((m) => ({ default: m.MarginLab }))) : null;

export function App() {
  // A development page for the margin-notes layout, at /?lab=margin. It exists only under
  // `npm run dev`: in a production build the condition is false and the page is dropped.
  if (Lab && new URLSearchParams(location.search).get("lab") === "margin") {
    return (
      <Suspense fallback={null}>
        <Lab />
      </Suspense>
    );
  }
  return <Main />;
}

function Main() {
  const [health, setHealth] = useState<Health | null>(null);
  const [me, setMe] = useState<Employee | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [slow, setSlow] = useState(false);
  const [open, setOpen] = useState<{ c: Citation; from: HTMLElement } | null>(null);

  const load = useCallback(async () => {
    // A free-tier server that has been idle takes up to a minute to wake, and until it does the
    // request is refused or times out. Keep trying every few seconds for about 90 seconds before
    // saying the server cannot be reached, so the first visit shows a waiting page, not a broken one.
    for (let attempt = 1; ; attempt++) {
      try {
        setHealth(await api<Health>("/api/health"));
        break;
      } catch (e) {
        const waking = e instanceof ApiError && (e.status === 0 || e.status >= 502);
        if (waking && attempt < 24) {
          await new Promise((r) => window.setTimeout(r, 4000));
          continue;
        }
        setError(e instanceof Error ? e.message : "Something went wrong.");
        return;
      }
    }
    if (tokenStore.get()) {
      try {
        setMe(await api<Employee>("/api/me"));
      } catch (e) {
        if (e instanceof ApiError && e.status === 401) tokenStore.clear();
      }
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  // A free-tier server sleeps. Say so when the first answer is slow, instead of leaving a blank page.
  useEffect(() => {
    if (health || error) return;
    const id = window.setTimeout(() => setSlow(true), 3000);
    return () => window.clearTimeout(id);
  }, [health, error]);

  const signOut = () => {
    tokenStore.clear();
    setMe(null);
  };

  if (!health)
    return (
      <Shell me={null} health={null} onSignOut={signOut}>
        <main id="main" className="shell-note">
          {error ? (
            <>
              <h1>Keystone could not be reached</h1>
              <p role="alert">{error}</p>
            </>
          ) : (
            <>
              <h1>Connecting to the server</h1>
              <p role="status">{slow ? "The server is waking up. On the free plan this can take up to a minute." : "One moment."}</p>
            </>
          )}
        </main>
      </Shell>
    );

  if (!me)
    return (
      <Shell me={null} health={health} onSignOut={signOut}>
        <AuthScreen
          health={health}
          onSignedIn={(e) => {
            setMe(e);
            void api<Health>("/api/health").then(setHealth);
          }}
        />
      </Shell>
    );

  const askOn = health.capabilities.embeddings && health.capabilities.answers;
  const off = !health.capabilities.embeddings
    ? "Questions are switched off on this deployment: it has no embeddings key."
    : "Questions are switched off on this deployment: it has no answering key.";

  return (
    <Shell me={me} health={health} onSignOut={signOut}>
      {me.role === "admin" ? (
        <AdminApp health={health} />
      ) : (
        <AskScreen me={me} canAsk={askOn} offReason={off} onOpen={(c, from) => setOpen({ c, from })} />
      )}
      {open && <Viewer key={open.c.chunk_id} citation={open.c} from={open.from} onClose={() => setOpen(null)} />}
    </Shell>
  );
}
