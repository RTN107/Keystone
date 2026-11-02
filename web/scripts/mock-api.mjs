// A stand-in for the Keystone API, for developing the interface. It speaks the real
// contract (JSON errors before the stream, then waiting/start/token/done events) and
// replays REAL answers captured from the container run, so layout and timing work is
// done on real content. It is not a test of the server: the real stack is driven in
// the container run. Usage: node scripts/mock-api.mjs   (port 8080)
//
// Dev controls:  POST /mock/config {waits, step, firstTokenMs, tokenMs}
//                POST /mock/extend   grants Delivery the playbook (as the admin screen will)
//                POST /mock/reset
import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const fx = (n) => JSON.parse(fs.readFileSync(path.join(here, "../src/fixtures", n + ".json"), "utf8"));
const fixtures = { p1: fx("p1-breach"), freeze: fx("deploy-freeze"), approval: fx("discount-approval"), retainer: fx("retainer-discount-granted") };
const docsDir = path.join(here, "../../demo-docs");
const docFiles = { 1: "pricing-and-proposal-playbook.md", 2: "leave-and-benefits-policy.md", 3: "delivery-and-escalation-sop.md" };
const content = (id) => fs.readFileSync(path.join(docsDir, docFiles[id]), "utf8").trim();

// The fixtures' offsets must index the document text exactly as the browser will slice it.
for (const f of Object.values(fixtures))
  for (const c of f.done.citations) {
    const u16 = content(c.document_id);
    if (u16.slice(c.char_start, c.char_end) !== c.passage) console.warn(`fixture passage ${c.chunk_id} no longer matches its document`);
  }

const people = {
  "DEL-001": { id: 2, employee_number: "DEL-001", name: "Asha Rao", role: "employee", team: { id: 2, name: "Delivery" } },
  "SAL-001": { id: 3, employee_number: "SAL-001", name: "Sam Iyer", role: "employee", team: { id: 1, name: "Sales & Marketing" } },
  "HR-001": { id: 4, employee_number: "HR-001", name: "Hana Nair", role: "employee", team: { id: 3, name: "HR" } },
  "ADM-001": { id: 1, employee_number: "ADM-001", name: "Admin", role: "admin", team: null },
};
const cfg0 = { waits: 0, step: 20, firstTokenMs: 900, tokenMs: 32 };
let cfg = { ...cfg0 };
let extended = false;
let asks = 0;

const refusal = (team) => `This is outside your team's access. The closest match to this question is in a document that Sales & Marketing holds and ${team} cannot read, so nothing from it was used here. To have Keystone use it, ask an admin to extend that document's access to your team.`;
const noMatch = "Keystone could not find an answer to this in the documents your team can read. Nothing relevant came back, so nothing was written. Try rewording the question, or ask an admin whether the document you need has been added for your team.";
const waitMsg = (attempt, max, step) =>
  attempt <= 1
    ? `Keystone is waiting its turn. The search service behind it allows only a few requests a minute on this plan, and this minute's are used up. Nothing has gone wrong and your question is still in line. Keystone will try again in about ${step} seconds, and this usually clears within a minute.`
    : `Still waiting its turn. The limit has not reset yet. Keystone will try again in about ${step} seconds and will keep trying for up to about ${(max - attempt) * step} seconds more. Your question is still in line.`;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const send = (res, status, body) => {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
};
const err = (res, status, code, message) => send(res, status, { error: { code, message } });
const body = (req) => new Promise((r) => { let b = ""; req.on("data", (c) => (b += c)); req.on("end", () => r(b ? JSON.parse(b) : {})); });
const who = (req) => Object.values(people).find((p) => `Bearer mock-${p.employee_number}` === req.headers.authorization);
const norm = (s) => s.trim().toLowerCase().split(/\s+/).join(" ");

function route(q, p) {
  const n = norm(q);
  if (n.includes("boom")) return { boom: true };
  if (n.includes("priority-one")) return { f: fixtures.p1 };
  if (n.includes("deploy to production")) return { f: fixtures.freeze };
  if (n.includes("discount above 10")) return p.team?.name === "Sales & Marketing" ? { f: fixtures.approval } : { blocked: true };
  if (n.includes("twelve month retainer")) return p.team?.name === "Sales & Marketing" || extended ? { f: fixtures.retainer } : { blocked: true };
  return { none: true };
}

async function ask(req, res, p) {
  const { question = "" } = await body(req);
  if (!question.trim()) return err(res, 400, "empty_question", "Type a question first.");
  res.writeHead(200, { "Content-Type": "text/event-stream", "Cache-Control": "no-cache" });
  const ev = (name, data) => res.write(`event: ${name}\ndata: ${JSON.stringify(data)}\n\n`);
  const r = route(question, p);
  const slow = cfg.waits > 0 && asks++ >= 0;
  for (let a = 1; slow && a <= cfg.waits; a++) {
    ev("waiting", { reason: "embeddings_rate_limited", attempt: a, max_attempts: 5, retry_in_seconds: cfg.step, message: waitMsg(a, 5, cfg.step) });
    await sleep(cfg.step * 1000);
  }
  if (r.boom) {
    ev("start", { outcome: "answered" });
    await sleep(cfg.firstTokenMs);
    for (const t of fixtures.p1.tokens.slice(0, 14)) {
      ev("token", { text: t });
      await sleep(cfg.tokenMs);
    }
    ev("error", { code: "answer_failed", message: "The answer stopped partway because Keystone lost its connection to the writing service. What is shown above is incomplete. Ask again in a moment.", partial: true });
  } else if (r.blocked) {
    ev("start", { outcome: "blocked" });
    ev("done", { outcome: "blocked", message: refusal(p.team?.name ?? ""), holding_teams: ["Sales & Marketing"], your_team: p.team?.name, citations: [] });
  } else if (r.none) {
    ev("start", { outcome: "no_match" });
    ev("done", { outcome: "no_match", message: noMatch, citations: [] });
  } else {
    ev("start", { outcome: "answered" });
    await sleep(cfg.firstTokenMs);
    for (const t of r.f.tokens) {
      ev("token", { text: t });
      await sleep(cfg.tokenMs);
    }
    ev("done", { ...r.f.done, truncated: false });
  }
  res.end();
}

http
  .createServer(async (req, res) => {
    res.setHeader("Access-Control-Allow-Origin", req.headers.origin ?? "*");
    res.setHeader("Access-Control-Allow-Headers", "Authorization, Content-Type");
    res.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
    if (req.method === "OPTIONS") return res.writeHead(204).end();
    const url = new URL(req.url, "http://x");
    if (url.pathname === "/mock/config") return (cfg = { ...cfg, ...(await body(req)) }), send(res, 200, cfg);
    if (url.pathname === "/mock/extend") return (extended = true), send(res, 200, { extended });
    if (url.pathname === "/mock/reset") return (cfg = { ...cfg0 }), (extended = false), (asks = 0), send(res, 200, {});
    if (url.pathname === "/api/health") return send(res, 200, { status: "ok", read_only: false, needs_setup: false, capabilities: { embeddings: true, answers: true } });
    if (url.pathname === "/api/auth/login") {
      const { employee_number = "" } = await body(req);
      const p = people[employee_number.trim().toUpperCase()];
      return p ? send(res, 200, { token: "mock-" + p.employee_number, expires_at: new Date(Date.now() + 36e5).toISOString(), employee: p }) : err(res, 401, "invalid_credentials", "That employee number and password do not match.");
    }
    const p = who(req);
    if (!p) return err(res, 401, "unauthenticated", "Sign in to continue.");
    if (url.pathname === "/api/me") return send(res, 200, p);
    if (url.pathname === "/api/ask" && req.method === "POST") return ask(req, res, p);
    const m = /^\/api\/documents\/(\d+)$/.exec(url.pathname);
    if (m) {
      const id = Number(m[1]);
      if (!docFiles[id]) return err(res, 404, "not_found", "That document is not available to your team.");
      const team = p.team?.name;
      const holder = { 1: "Sales & Marketing", 2: "HR", 3: "Delivery" }[id];
      if (holder !== team && !(id === 1 && extended && team === "Delivery")) return err(res, 404, "not_found", "That document is not available to your team.");
      const c = content(id);
      return send(res, 200, { id, title: c.split("\n")[0].replace(/^#\s*/, ""), content: c });
    }
    err(res, 404, "not_found", "That endpoint does not exist.");
  })
  .listen(Number(process.env.PORT ?? 8080), "127.0.0.1", () => console.log("mock api on 8080"));
