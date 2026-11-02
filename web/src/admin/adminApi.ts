import { api, type Employee } from "../api";

export type TeamTag = { id: number; name: string; source: "ingest" | "extension" };
export type Doc = { id: number; title: string; source_filename: string; created_at: string; ingested_by: string | null; passages: number; teams: TeamTag[] };
export type TeamWithMembers = { id: number; name: string; number_prefix: string; members: Employee[] };
export type LogEntry = { document_id: number; title: string; source_filename: string; at: string; by: string | null; passages: number; teams: string[] };
export type Outcome = "answered" | "blocked" | "no_match";
export type LoggedQuery = {
  id: number;
  at: string;
  employee_number: string;
  employee_name: string;
  team_id: number;
  team: string;
  question: string;
  outcome: Outcome;
  answer: string | null;
  cited_chunk_ids: number[];
};
export type Uploaded = { document_id: number; title: string; passages: number; tokens: number; teams: string[]; message: string };
export type AccessResult = { document_id: number; title: string; team: { id: number; name: string }; message: string };

// A list that is not a list means the page and the server disagree. Say so, instead of showing an empty screen.
function list<T>(v: T[] | undefined, what: string): T[] {
  if (!Array.isArray(v)) throw new Error(`Keystone returned the ${what} in a form this page did not expect. Reload the page, and if it persists the app and the server are out of step.`);
  return v;
}

export const listDocuments = () => api<{ documents: Doc[] }>("/api/admin/documents").then((r) => list(r.documents, "documents"));
export const listTeams = () => api<{ teams: TeamWithMembers[] }>("/api/admin/teams").then((r) => list(r.teams, "teams"));
export const listIngestionLog = () => api<{ entries: LogEntry[] }>("/api/admin/ingestion-log").then((r) => list(r.entries, "ingestion log"));
export const listQueryLogs = (f: { teamId?: number; employee?: string; limit: number }) => {
  const q = new URLSearchParams({ limit: String(f.limit) });
  if (f.teamId) q.set("team_id", String(f.teamId));
  if (f.employee) q.set("employee", f.employee);
  return api<{ entries: LoggedQuery[] }>(`/api/admin/query-logs?${q}`).then((r) => list(r.entries, "query log"));
};
export const uploadDocument = (file: File, teamIds: number[], title: string) => {
  const f = new FormData();
  f.append("file", file);
  for (const id of teamIds) f.append("team_id", String(id));
  if (title.trim()) f.append("title", title.trim());
  return api<Uploaded>("/api/admin/documents", { method: "POST", body: f });
};
export type DeleteResult = { document_id: number; title: string; passages: number; questions_removed: number; message: string };
export const deleteDocument = (docId: number) => api<DeleteResult>(`/api/admin/documents/${docId}`, { method: "DELETE" });
export const extendAccess = (docId: number, teamId: number) => api<AccessResult>(`/api/admin/documents/${docId}/access`, { method: "POST", body: { team_id: teamId } });
export const revokeAccess = (docId: number, teamId: number) => api<AccessResult>(`/api/admin/documents/${docId}/access/${teamId}`, { method: "DELETE" });
export const createEmployee = (b: { name: string; team_id: number; password: string; employee_number?: string }) => api<Employee>("/api/admin/employees", { method: "POST", body: b });
