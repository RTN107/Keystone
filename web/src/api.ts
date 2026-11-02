// The API base comes from the build, so the same bundle can point at a local
// container or a hosted one.
const BASE = (import.meta.env.VITE_API_URL ?? "http://localhost:8080").replace(/\/$/, "");

export type Team = { id: number; name: string };
export type Employee = {
  id: number;
  employee_number: string;
  name: string;
  role: "admin" | "employee";
  team: Team | null;
};
export type Session = { token: string; expires_at: string; employee: Employee };
export type Health = {
  status: string;
  read_only: boolean;
  needs_setup: boolean;
  capabilities: { embeddings: boolean; answers: boolean };
};

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

// sessionStorage can throw (private windows, blocked storage), so every access is guarded.
const KEY = "keystone.token";
export const tokenStore = {
  get(): string | null {
    try {
      return sessionStorage.getItem(KEY);
    } catch {
      return null;
    }
  },
  set(token: string) {
    try {
      sessionStorage.setItem(KEY, token);
    } catch {
      /* the session then lasts until the page reloads */
    }
  },
  clear() {
    try {
      sessionStorage.removeItem(KEY);
    } catch {
      /* nothing to clear */
    }
  },
};

export async function api<T>(path: string, init: { method?: string; body?: unknown; token?: string | null } = {}): Promise<T> {
  const headers: Record<string, string> = {};
  // A FormData body (an upload) sets its own Content-Type, boundary included.
  const form = typeof FormData !== "undefined" && init.body instanceof FormData;
  if (init.body !== undefined && !form) headers["Content-Type"] = "application/json";
  const token = init.token === undefined ? tokenStore.get() : init.token;
  if (token) headers.Authorization = `Bearer ${token}`;

  let res: Response;
  try {
    res = await fetch(BASE + path, {
      method: init.method ?? "GET",
      headers,
      body: init.body === undefined ? undefined : form ? (init.body as FormData) : JSON.stringify(init.body),
    });
  } catch {
    throw new ApiError(0, "unreachable", "Keystone could not reach the server. Check your connection, or wait a moment if it is waking up, then try again.");
  }
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(res.status, data?.error?.code ?? "error", data?.error?.message ?? "Something went wrong. Try again in a moment.");
  }
  return data as T;
}
