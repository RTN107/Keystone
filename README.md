<p align="center">
  <img src="docs/images/hero.png" alt="Keystone answering a Delivery employee's question about a priority-one ticket that breaches its SLA: a numbered three-step answer in the middle, and the source it came from, a Delivery Handbook passage, in the margin on the right" width="820" />
</p>

<p align="center">
  <a href="#1-try-it-in-five-minutes">Try it</a> •
  <a href="#2-what-you-need">What you need</a> •
  <a href="#3-setting-it-up">Setup</a> •
  <a href="#4-where-the-secrets-go">Secrets</a> •
  <a href="#5-making-it-yours">Making it yours</a> •
  <a href="#6-deploying-it-as-a-live-website">Deploying it</a> •
  <a href="#7-how-it-is-put-together">Structure</a> •
  <a href="#8-limits">Limits</a>
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/React-61DAFB?logo=react&logoColor=black" />
  <img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-3178C6?logo=typescript&logoColor=white" />
  <img alt="PostgreSQL" src="https://img.shields.io/badge/PostgreSQL-4169E1?logo=postgresql&logoColor=white" />
  <img alt="Vite" src="https://img.shields.io/badge/Vite-646CFF?logo=vite&logoColor=white" />
  <img alt="Docker" src="https://img.shields.io/badge/Docker-2496ED?logo=docker&logoColor=white" />
</p>

# Keystone

A role-based internal knowledge agent. People ask questions in plain language and get cited answers drawn only from the documents their team is allowed to read. When the best match for a question is in a document their team cannot read, Keystone says so and names the team that holds it, and the model is never shown that document. Go on the backend, React and TypeScript on the frontend, PostgreSQL with pgvector for storage and search, Voyage AI for embeddings and Claude for the answers, all runnable on your machine with Docker Compose.

The idea that matters is where the permission check lives. Who may read what is a condition inside the database query that searches the documents, not a filter applied to the results afterwards. A passage a team cannot read is never returned to the application, so no later piece of code can forget to remove it, and nothing from it can reach the model or the browser. It suits any organisation whose documents have different audiences: HR, finance, legal, support, engineering, sales.

**This repository ships with no company data.** You define your own teams, add your own people and upload your own documents. Three fictional demo documents are included in `demo-docs/` so you can see it work before you have your own, and every screenshot on this page uses them. Nothing about the demo company is in the code. The teams in `.env.example` (Sales & Marketing, Delivery, HR) are samples written for those demo documents, and section 5.1 shows how to replace them.

---

## 1. Try it in five minutes

You need Docker with Compose. For the full flow you also need two API keys (section 2). Without them everything except ingesting documents and writing answers works, and the interface says so.

```
git clone https://github.com/RTN107/Keystone.git
cd Keystone
cp .env.example .env
```

Open `.env` and set two values:

- `JWT_SECRET` to any long random string, for example the output of `openssl rand -hex 32`
- `POSTGRES_PASSWORD` to any password made of letters and digits, for example the output of `openssl rand -hex 16`

For answers, also set `VOYAGE_API_KEY`, `KEYSTONE_ANTHROPIC_KEY` and `ANTHROPIC_MODEL` (section 3.2 explains each). Then:

```
docker compose up --build
```

- Web app: http://localhost:5173
- API health check: http://localhost:8080/api/health

The first time you open the web app the database is empty, so it asks you to create the first admin account instead of showing a sign-in screen. That account is `ADM-001`.

<p align="center">
  <img src="docs/images/first-run.png" alt="Keystone's first screen on a new install: a form that creates the administrator account, with a name and a password of at least 10 characters" width="820" />
</p>

From there, with the sample teams and demo documents:

1. **People.** Add one employee to each team. Leave the employee number empty and Keystone assigns the next one for that team (`SAL-001`, `DEL-001`, `HR-001`). The line that appears after you add someone tells you the number to sign in with. Passwords need at least 10 characters.
2. **Documents.** Upload `demo-docs/pricing-and-proposal-playbook.md` for Sales & Marketing only, `demo-docs/leave-and-benefits-policy.md` for HR only and `demo-docs/delivery-and-escalation-sop.md` for Delivery only. Each is split into passages and embedded, which takes a second or two. If your embeddings account is rate limited, an upload waits and says it is working.

<p align="center">
  <img src="docs/images/upload-result.png" alt="The admin Documents page after uploading the Delivery document: 26 passages were embedded, and the document is listed as readable by Delivery only" width="820" />
</p>

3. **Ask.** Sign out, then sign in as the Delivery employee and ask "What discount can I offer a client who commits to a twelve month retainer?" The answer is in a Sales & Marketing document, so Keystone refuses, names the team that holds it and shows an empty keystone-shaped slot where a source would have been.

<p align="center">
  <img src="docs/images/refused.png" alt="A Delivery employee is refused: the message says the closest match is in a document that Sales & Marketing holds and Delivery cannot read, and a slot in the margin reads Held by Sales & Marketing, Not open to your team" width="820" />
</p>

4. **Give access.** Sign in as the admin, open the playbook's Access page, choose Delivery and confirm. The page spells out what will change before you press the button.

<p align="center">
  <img src="docs/images/access-grant.png" alt="The admin Access page for a document: a confirmation reading that Delivery can now read it, a list of the teams that can read it with their source (given at upload, or extended after), a form to give another team access, and a delete section" width="820" />
</p>

5. **Ask again.** The same question, from the same employee, is now answered from the document, with numbered citations in the margin. The slot fills.

<p align="center">
  <img src="docs/images/answered-after-grant.png" alt="The same question answered with a numbered answer, each claim carrying a citation marker, and the cited passages shown beneath it" width="820" />
</p>

Every citation opens the source document on the exact passage, marked within the whole document, so a reader can check the claim in context.

<p align="center">
  <img src="docs/images/citation-opened.png" alt="A citation opened: the source document slides in from the right with the cited passage highlighted among the passages around it" width="820" />
</p>

`demo-docs/README.md` lists questions to try and what each should show. The interface also works at phone size:

<p align="center">
  <img src="docs/images/mobile-refused.png" alt="A refusal on a phone" width="240" />
  &nbsp;&nbsp;&nbsp;
  <img src="docs/images/mobile-answered.png" alt="An answer with its sources inline on a phone" width="240" />
</p>

---

## 2. What you need

Only two things here can cost money, and both are API keys. Everything else is free, either open source or on a free plan. Plans and limits change, so these are what the providers' own documentation said in November 2026. Check them before you rely on them.

### 2.1 Accounts and keys

| Dependency | What it does here | Where it is used | Cost |
|---|---|---|---|
| [Voyage AI](https://www.voyageai.com/) API key | Turns documents and questions into vectors, so a question can be matched to passages by meaning | The `embed` package in the API, called when a document is ingested and when a question is asked | Free allowance of tokens, then paid. Free accounts are rate limited: the one used to build this allowed 3 requests a minute, and Keystone is built to wait and say so on screen |
| [Anthropic](https://www.anthropic.com/) API key | The Claude model that writes the cited answer | The `answer` package in the API, called only when a question is answered, never for a refusal | **Paid**, per token. New accounts may receive a small amount of free credit |

Both are optional. Without the Voyage key, documents cannot be ingested or searched. Without the Anthropic key, asking is switched off and the interface says so. Teams, people, access and the logs still work.

### 2.2 Tools on your machine

| Requirement | What for | Cost |
|---|---|---|
| Docker and Docker Compose | Runs the database, the API and the web app as three containers, with nothing else to install | Free |
| Go 1.26 or later | Only to run the API or its tests outside Docker | Free |
| Node 22 or later | Only to run the web app or its tests outside Docker | Free |

The database image is `pgvector/pgvector:pg17`, PostgreSQL 17 with the pgvector extension, which stores the vectors and does the similarity search.

### 2.3 Hosting, only if you deploy it (section 6)

| Service | What it hosts | Free plan, and the catch |
|---|---|---|
| [Neon](https://neon.com/) | The PostgreSQL database | Free plan with 1 GB of storage per project. The database suspends after 5 minutes without activity |
| [Render](https://render.com/) | The Go API, as a Docker web service | Free web services sleep after 15 minutes without traffic and take about a minute to wake. 750 free instance hours a month |
| [Vercel](https://vercel.com/) | The built web app | The Hobby plan is free but limited to personal, non-commercial use |

Any host that runs a Docker container, a Postgres database with pgvector and a static site will do. These three are what this project was deployed on.

### 2.4 Libraries

All free and open source.

| Part | Libraries | What for |
|---|---|---|
| API | `pgx` (PostgreSQL driver), `goose` (schema migrations), `golang.org/x/crypto` (bcrypt password hashing), `golang-jwt` (session tokens), `ledongthuc/pdf` (text from PDFs) | Database access and migrations, sign-in, reading uploaded PDFs |
| Web | React, Vite, TypeScript, and the Hanken Grotesk and Literata variable fonts from `@fontsource` | The interface. There is no component kit and no router library |

---

## 3. Setting it up

### 3.1 Get the code

```
git clone https://github.com/RTN107/Keystone.git
cd Keystone
cp .env.example .env
```

### 3.2 Fill in `.env`

| Variable | What it controls | Can you leave it? |
|---|---|---|
| `JWT_SECRET` | Signs login sessions. At least 32 characters | No. Generate your own: `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | The local database's own password, used only between the `db` and `api` containers | No. Letters and digits only, because it is placed inside a URL: `openssl rand -hex 16` |
| `POSTGRES_USER`, `POSTGRES_DB` | The local database's user and database name | Yes, both default to `keystone` |
| `KEYSTONE_TEAMS` | The teams to create at startup, as `Name:PREFIX;Name:PREFIX` | Replace the samples with your own (section 5.1). If it is empty, no teams exist and nothing can be assigned to one |
| `VOYAGE_API_KEY` | Embeddings | Yes, but then documents cannot be ingested or searched |
| `VOYAGE_BASE_URL` | Where embedding calls go | Yes. The default is Voyage's API as served through MongoDB, which is what this was built and tested against. If your key was issued by voyageai.com, set it to `https://api.voyageai.com/v1`, the endpoint Voyage documents. That endpoint has not been tested here |
| `VOYAGE_MODEL` | The embedding model | Yes, the default is `voyage-4-lite`. A model you choose must be able to return 1024-dimension vectors (section 5.5) |
| `KEYSTONE_ANTHROPIC_KEY` | The answering model's key. `ANTHROPIC_API_KEY` is read as a fallback for hosts that reserve that name | Yes, but then answers are off |
| `ANTHROPIC_MODEL` | Which Claude model writes answers | **Required if you set a key.** There is no default, because model names are retired over time. Choose one from Anthropic's list of models |
| `KEYSTONE_RELEVANCE_FLOOR`, `KEYSTONE_BLOCK_MARGIN` | What counts as relevant, and when a question is refused | Yes, but read section 5.4 first |
| `KEYSTONE_WARM_QUESTIONS` | Questions to embed once at startup, separated by `\|` | Yes. Empty means none |
| `KEYSTONE_ASKS_PER_IP`, `KEYSTONE_DAILY_ASK_CAP`, `KEYSTONE_PROXY_HOPS` | Flood protection for a public link (section 5.7) | Yes. Defaults are 6 questions per address per 10 minutes, 150 questions a day for the whole deployment, and 1 trusted proxy |
| `KEYSTONE_READ_ONLY` | `true` refuses every change except signing in and asking | Yes, defaults to `false` |
| `DB_PORT`, `API_PORT`, `WEB_ORIGIN` | Local ports, and the web origin the API accepts | Yes. Uncomment them only if a default is already taken |

The web app's port, 5173, is fixed in `compose.yaml`. If something else is using it, `docker compose up` fails with a port-in-use error: stop the other program, or change the `5173:5173` line (and `WEB_ORIGIN` to match).

### 3.3 Run it

```
docker compose up --build
```

This starts three containers. `db` is Postgres with pgvector. `api` is the Go server, which applies every migration in `api/internal/db/migrations` on startup and creates the teams from `KEYSTONE_TEAMS`, so there is no separate setup step. `web` is the Vite development server. The first run builds the Go binary and installs the web dependencies, so it takes longer than later runs.

### 3.4 Create your organisation

Open http://localhost:5173 and create the admin. Then, in the admin screens:

| Screen | What you do there |
|---|---|
| **People** | Add employees to teams. Employee numbers and passwords are set here |
| **Documents** | Upload documents and choose which teams can read each. The list shows every document, its passage count and who can read it |
| **Ingestion log** | See every upload: who, when, how many passages, which teams |
| **Query log** | See every question asked, by whom, and what Keystone did with it |

There is no seed script and no fixture data. This is the only way data gets into a fresh database.

### 3.5 Running the pieces outside Docker

Only needed to edit the Go code with a debugger, or the web app without the container.

```
docker compose up db                  # just the database, on 127.0.0.1:5432
```

In one terminal, with your own values in the environment:

```
cd api
export DATABASE_URL="postgres://keystone:<your POSTGRES_PASSWORD>@localhost:5432/keystone?sslmode=disable"
export JWT_SECRET="<your JWT_SECRET>"
export KEYSTONE_TEAMS="Sales & Marketing:SAL;Delivery:DEL;HR:HR"
go run ./cmd/keystone
```

In another:

```
cd web
npm install
npm run dev
```

The API reads every setting listed in 3.2 from its environment, and the web app finds the API at `VITE_API_URL`, which defaults to `http://localhost:8080`.

### 3.6 Working on the interface without API keys

`web/scripts/mock-api.mjs` is a stand-in API that replays captured answers for the demo documents, so you can work on layout and timing with no keys and no database.

```
node web/scripts/mock-api.mjs         # listens on port 8080
cd web && npm run dev
```

Under `npm run dev`, http://localhost:5173/?lab=margin opens a test bench for the sources margin. Neither is part of a production build.

### 3.7 Tests

The API's database tests need a Postgres with pgvector and an empty database to use. They empty their own tables, so never point them at a database you care about.

```
docker run -d --name keystone-testdb -e POSTGRES_USER=keystone -e POSTGRES_PASSWORD=<any password> \
  -e POSTGRES_DB=keystone_test -p 5433:5432 pgvector/pgvector:pg17
cd api
TEST_DATABASE_URL='postgres://keystone:<that password>@localhost:5433/keystone_test?sslmode=disable' \
  go test -p 1 -count=1 ./...
cd ../web
npm ci
npm test                      # logic tests
node scripts/contrast.mjs     # colour contrast gate
npm run build                 # type check and production build
```

Run packages with `-p 1`, because they share one database. Two diagnostics spend real embedding requests and only run when asked, with `LIVE_EMBEDDINGS=1` and your Voyage variables set: the threshold calibration in `api/internal/retrieve` (`TestLiveCalibration`) and the retrieval check in `api/internal/ingest` (`TestLiveDemoRetrieval`). Both use the demo documents.

---

## 4. Where the secrets go

This repository contains no secrets. Everything lives in your own `.env`, which is git ignored, or in the environment variables of whatever host you deploy to in section 6.

| Variable | File | Replace with |
|---|---|---|
| `JWT_SECRET` | `.env` | A long random string: `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | `.env` | Any password of letters and digits: `openssl rand -hex 16` |
| `VOYAGE_API_KEY` | `.env`, or the host's environment | Your key from your embeddings account |
| `KEYSTONE_ANTHROPIC_KEY` | `.env`, or the host's environment | Your key from your Anthropic account |
| `DATABASE_URL`, `DIRECT_DATABASE_URL` | The host's environment | Your hosted database's connection strings. Compose sets its own for the local database |
| `WEB_ORIGIN` | `compose.yaml` default, or the host's environment | The web app's exact origin. The API accepts requests only from this one origin |
| `VITE_API_URL` | `compose.yaml`, or the web host's environment | The API's exact URL, read by the web app at build time |

Two habits that keep it that way. Never paste a key into an issue, a log or a screenshot. And remember that `docker compose` reads variables from your shell as well as from `.env`, so `docker compose config` prints any key your shell has exported.

The first admin account is created through the web app and its password is stored only as a bcrypt hash. Nothing in the repository is a default login.

---

## 5. Making it yours

The demo is a sample, and every part of it can be replaced. This section is where.

### 5.1 Teams

Teams are the unit of access: a document is readable by one or more teams, and an employee belongs to exactly one. You define them in `KEYSTONE_TEAMS`:

```
KEYSTONE_TEAMS="Support:SUP;Finance:FIN;Legal:LEG"
```

Each entry is `Name:PREFIX`. The prefix is 2 to 4 capital letters and starts that team's employee numbers (`SUP-001`, `SUP-002`). `ADM` is reserved for admins. Teams are created when the API starts, if they do not exist yet.

Restarting with a longer list adds the new teams. It never renames, reorders or removes one, and it refuses a name that already exists with a different prefix, or a prefix already used by another team, because existing employee numbers carry the old prefix. To retire a team, leave it in the list and stop assigning it people and documents.

<p align="center">
  <img src="docs/images/people.png" alt="The People page: each team listed with its employees and their numbers, and a form to add an employee with a name, a team, an optional employee number and a password" width="820" />
</p>

### 5.2 People and roles

There are two roles.

| Role | Can do |
|---|---|
| `admin` | Sign in to the admin screens: documents, access, people, both logs. Belongs to no team and cannot use Ask |
| `employee` | Ask questions. Reads only the documents their team can read. Belongs to exactly one team |

The first admin is created on first run. After that, the admin adds employees on the People screen.

### 5.3 Documents and access

Upload `.md`, `.txt` or text-based `.pdf` files up to 5 MiB. A scanned PDF has no selectable text, and Keystone says so and asks for a text version. Text is split into passages along paragraphs, roughly 350 to 600 characters each, with a heading kept together with the paragraph under it. Each passage is embedded and stored. Markdown tables are kept as tables, and a cited table is shown as a table.

<p align="center">
  <img src="docs/images/citation-opened-table.png" alt="A cited table opened in the source document: the response targets table is shown as a table, with its whole section marked" width="820" />
</p>

<p align="center">
  <img src="docs/images/documents.png" alt="The Documents page: every document with its title, source file, passage count, who uploaded it and which teams can read it, and the form for uploading a new one beneath" width="820" />
</p>

When you upload, you choose the teams that can read the document. Afterwards, each document's Access page does three things:

- **Give another team access.** The page names the consequence before you confirm.
- **Take back** an extension you gave. The team the document was uploaded for keeps it.
- **Delete** the document, after a second confirmation. It removes the passages, the access rows, the upload record and every logged question whose answer cited it.

<p align="center">
  <img src="docs/images/access-holders.png" alt="The Access page showing the teams that can read a document, with the source of each: given at upload, or extended after" width="410" />
  <img src="docs/images/delete-confirm.png" alt="The delete confirmation, which says exactly what will be removed and offers to keep the document" width="410" />
</p>

There is no edit. To change a document, delete it and upload the new version. Identity ids are never reused, so after deleting you will see a gap in the ids.

The Ingestion log records each upload, and the Query log records each question and what Keystone did with it, with filters by team, employee and outcome.

<p align="center">
  <img src="docs/images/ingestion-log.png" alt="The Ingestion log: each upload with its time, who ingested it, the number of passages and the teams that can read it" width="410" />
  <img src="docs/images/query-log-blocked.png" alt="The Query log filtered to blocked questions: each row shows when, who asked, from which team, the question and the outcome" width="410" />
</p>

### 5.4 Tuning the thresholds

Keystone decides what to do with a question from two numbers, both cosine similarities between the question and a passage.

- The **relevance floor** (default `0.40`): a passage scoring below it is not treated as relevant.
- The **margin** (default `0.05`): how far the best passage the asker may not read must beat the best passage they may read before the question counts as blocked.

The decision is plain code over those numbers: blocked if the best forbidden passage clears the floor and beats the best permitted one by the margin; otherwise answered if the best permitted passage clears the floor; otherwise no match. Blocked and no-match replies are fixed text and never call the model.

**The defaults were measured on the three demo documents with `voyage-4-lite`, and nothing else.** Your documents, your model and your language will score differently, so treat them as a starting point. To tune for your own:

1. Write 30 or more questions that should be answered, some that should be blocked for a given team, and some that no document answers.
2. Ask them as employees of different teams. For every question the API logs one `ask decided` line with `best_permitted`, `best_forbidden` and the outcome: `docker compose logs api`.
3. Find a floor that separates the answerable questions from the unanswerable ones as well as it can, then a margin that separates blocked from answered. They will overlap somewhere. That overlap is the nature of similarity scores, and section 8 says what it looks like.
4. Set `KEYSTONE_RELEVANCE_FLOOR` and `KEYSTONE_BLOCK_MARGIN` and restart. No rebuild is needed. Invalid values stop startup with a message rather than falling back silently.

<p align="center">
  <img src="docs/images/no-match.png" alt="A question no document answers: Keystone says it could not find an answer in the documents the team can read, and suggests rewording or asking an admin" width="820" />
</p>

### 5.5 Models

**Answers** use the Claude model named in `ANTHROPIC_MODEL`. The call is streamed, answers are capped at 1,000 tokens, and the cap is the `MaxTokens` constant in `api/internal/answer/prompt.go`. To use a different answering provider, implement the small `answer.Model` interface and construct it in `api/cmd/keystone/main.go`.

**Embeddings** use `VOYAGE_MODEL` (default `voyage-4-lite`) at 1024 dimensions. Two things follow if you change them.

- The database column is `vector(1024)`, set in `api/internal/db/migrations/00002_document_chunks.sql`. A model must be able to return 1024-dimension vectors to fit, and Keystone refuses a response of any other size.
- Vectors from different models are not comparable. After changing the model, or the provider, delete and re-ingest every document, and re-tune the thresholds from section 5.4. For a model with a different size, write a new migration for the column and change the `Dimension` constant in `api/internal/embed`.

To see what a model really returns before you commit to it, run the diagnostic. It prints the model's default vector size and what it returns when a size is requested, and never prints your key. On a free account add `EMBEDCHECK_PAUSE_SECONDS=21` to stay under the request limit.

```
cd api
VOYAGE_API_KEY=<your key> VOYAGE_MODEL=<model to check> go run ./cmd/embedcheck
```

### 5.6 What the model is told, and what it never says

The model's instructions are the `systemPrompt` constant in `api/internal/answer/prompt.go`. It tells the model to answer only from the supplied passages, to cite each claim as `[n]`, to say plainly what the passages do not cover, and to treat everything inside the passage and question tags as data and not as instructions. Edit it for your tone and your domain.

Everything Keystone says without the model is in `api/internal/answer/copy.go`: the refusal, the no-match message, the waiting messages and the errors. The refusal names the holding team and nothing else about the document. Reword them there.

A citation marker is trusted only if it names a passage that was actually supplied. A made-up `[9]` never reaches the browser.

### 5.7 Protecting a public link

If you put a deployment on the open internet, every question costs an embedding call and a model call. Three limits, all in memory, protect that.

| Setting | Default | What it limits |
|---|---|---|
| `KEYSTONE_ASKS_PER_IP` | 6 | Questions one client address may ask in 10 minutes |
| (fixed) | 300 | Questions across the deployment in an hour, a brake for bursts |
| `KEYSTONE_DAILY_ASK_CAP` | 150 | Questions across the deployment in 24 hours, after which Keystone rests and says so |

The client address is read from `X-Forwarded-For`, counting `KEYSTONE_PROXY_HOPS` entries from the right, because the entries on the left are written by the client and can be forged. Set it to the number of trusted proxies in front of the API. The default of 1 suits a single proxy. Behind two, such as a CDN in front of a host's own proxy, set 2. If the per-address limit seems not to trigger, look at the `client` and `xff_entries` fields of the `ask decided` log line.

`KEYSTONE_READ_ONLY=true` refuses every change except signing in and asking: no uploads, no deletes, no access changes, no new accounts. Every control that would change something is switched off and says why. Read-only mode does not hide anything from an admin, so do not share admin credentials with the public.

<p align="center">
  <img src="docs/images/read-only.png" alt="The Documents page on a read-only deployment: a notice at the top says changes are switched off, and the upload form is disabled with a line explaining why" width="820" />
</p>

### 5.8 Branding

`web/src/styles.css` holds every colour, font and size as variables. `web/src/ui/stoneGeometry.ts` is the single source of the mark, the empty and filled slot, and the favicon: edit the geometry there, then run `node web/scripts/build-icons.mjs` to regenerate the SVG icons. `web/index.html` holds the page title. Keep the contrast gate (`node web/scripts/contrast.mjs`) passing after any colour change: it is part of the web build checks.

---

## 6. Deploying it as a live website

Section 1 runs it on your machine. This puts it on the internet, on three free plans (section 2.3): the database on Neon, the API on Render, the web app on Vercel.

```
Vercel (web app, Vite build of web/)
   |
   v  VITE_API_URL
Render (API, Dockerfile in api/)
   |
   v  DATABASE_URL
Neon (Postgres with pgvector)
```

The API reads every setting from environment variables and applies its own migrations on startup, so a fresh deploy needs no manual database work.

### 6.1 Database: Neon

1. Create a Neon project and a database.
2. Press **Connect** and copy the **pooled** connection string. It looks like `postgresql://user:password@host-pooler.region.aws.neon.tech/dbname?sslmode=require`.
3. Optionally copy the **direct** string too. Migrations take a session-level lock that does not survive a connection pooler, so they run on the direct host. If you give only the pooled string, Keystone derives the direct host by removing `-pooler` from the host name.

pgvector needs no setup: the first migration runs `CREATE EXTENSION IF NOT EXISTS vector`. The API talks to Neon's pooled endpoint with PostgreSQL's simple query protocol, because that endpoint is a PgBouncer.

### 6.2 API: Render

1. **New, Web Service**, connect your fork of this repository, language **Docker**.
2. Leave **Root Directory** empty. Set **Dockerfile Path** to `./api/Dockerfile` and **Docker Build Context** to `./api`.
3. Choose the **Free** instance type, and set the **Health Check Path** to `/api/health`.
4. Set these environment variables:

   | Variable | Value |
   |---|---|
   | `DATABASE_URL` | The Neon pooled string |
   | `DIRECT_DATABASE_URL` | The Neon direct string (optional, see 6.1) |
   | `JWT_SECRET` | A long random string, for example `openssl rand -hex 32` |
   | `KEYSTONE_TEAMS` | Your teams, as in 5.1 |
   | `VOYAGE_API_KEY`, `KEYSTONE_ANTHROPIC_KEY`, `ANTHROPIC_MODEL` | As in section 3.2 |
   | `WEB_ORIGIN` | The Vercel URL from 6.3, once you have it |

5. Deploy. Render gives you a URL like `https://your-service.onrender.com` and passes its own `PORT`, which the API uses automatically. Check that `https://<that-url>/api/health` returns `"status":"ok"`.

### 6.3 Web app: Vercel

1. **Add New, Project**, import your fork. Set the framework preset to **Vite** and the **Root Directory** to `web`. The build command is `npm run build` and the output directory is `dist`.
2. Set one environment variable, `VITE_API_URL`, to the Render URL from 6.2 with no trailing slash.
3. Deploy. `web/vercel.json` rewrites every path to the single page.

### 6.4 Close the loop, and create the admin

Go back to Render and set `WEB_ORIGIN` to the exact Vercel origin, with no path and no trailing slash. The API accepts requests only from that origin, so this step is what lets the web app talk to it.

Then open the Vercel URL **immediately** and create the admin. Until the first admin exists, anyone who finds the address can create it.

### 6.5 Things to know

- Render redeploys on every push to the branch it tracks, and so does Vercel. Batch your pushes.
- The free API host sleeps when idle. The first request after a quiet period takes up to about a minute, and the web app shows "The server is waking up" and keeps retrying while it does. The database suspends when idle too, and wakes with the first query.
- The public-link protections in section 5.7 are on by default. Consider `KEYSTONE_READ_ONLY=true` for a showcase deployment.
- Keystone has no password reset or change. An admin who forgets their password, or an employee whose password you want to change, needs a hash written to the database by hand. Generate one with the same bcrypt cost the API uses (12). Keep the first admin's password somewhere safe.

---

## 7. How it is put together

<p align="center">
  <img src="docs/images/stack.png" alt="The stack as four cards from left to right: React and TypeScript hosted on Vercel, Go hosted on Render, Neon with Postgres and pgvector, and Voyage with Claude used as services. Beneath them: access is checked inside the database search, and forbidden passages never reach the model" width="820" />
</p>

```
Keystone/
├── api/                             The Go API
│   ├── cmd/
│   │   ├── keystone/main.go         Entry point: load config, migrate, create teams, serve
│   │   └── embedcheck/              Checks what an embeddings model returns (section 5.5)
│   ├── internal/
│   │   ├── config/                  Every setting, read once from the environment
│   │   ├── db/migrations/           Every schema change, applied once each, in order, on startup
│   │   ├── auth/                    Password hashing, session tokens, the sign-in throttle
│   │   ├── store/                   Teams, people, access, query logs
│   │   ├── ingest/                  Extract text, split into passages, embed, store; delete
│   │   ├── retrieve/                The team-scoped search, the thresholds and the decision
│   │   ├── embed/                   The embeddings client: batching, retries, one call at a time
│   │   ├── answer/                  The prompt, the streamed model call, citation checking, fixed copy
│   │   ├── httpapi/                 Handlers, the router, access and read-only middleware
│   │   ├── ratelimit/               The in-memory limiter used for asking
│   │   ├── archtest/                Tests that fail the build if the access design is bypassed
│   │   └── testutil/                A test database and fake embeddings
│   └── Dockerfile
├── web/                             The React app
│   ├── src/
│   │   ├── ask/                     The employee screen: question, streamed answer, sources margin, viewer
│   │   ├── admin/                   Documents, Access, Ingestion log, Query log, People
│   │   ├── auth/                    Sign-in and the first-run admin screen
│   │   ├── ui/                      The mark and the slot, from one geometry file
│   │   ├── api.ts                   The one place requests to the API are made
│   │   └── styles.css               Colours, fonts and sizes as variables
│   ├── scripts/                     Contrast gate, icon builder, the mock API
│   ├── tests/                       Logic tests
│   └── Dockerfile                   A development image. The hosted app is built by Vercel
├── demo-docs/                       Three fictional documents, and what to ask them
├── docs/images/                     The images used in this file
├── compose.yaml                     The three containers for section 1
└── .env.example                     Every variable section 3.2 explains
```

### How a question is answered

1. The question is embedded.
2. One SQL statement searches the passages, with the asking team's permission as a condition on the rows it may consider (a `WHERE EXISTS` on `document_access`). A passage the team cannot read is never returned to application code.
3. To tell "refused" from "no answer exists", a second, separate query looks only at the documents the team cannot read. It returns two things and nothing else: the best similarity score and the names of the teams that hold the document.
4. Plain code decides between answered, blocked and no match, using the two thresholds.
5. Blocked and no match are fixed text. Only an answered question reaches the model, with only permitted passages, and the answer streams back with its citations.

This is tested, not just asserted. A canary test plants unique strings in each team's documents, asks leading questions as every team, and fails if a canary reaches the model's prompt or any response. An architecture test fails the build if any package other than retrieval and ingestion touches the passages table, or if a retrieval query lacks the permission condition. The citation viewer uses the same permission rule as search.

---

## 8. Limits

These are the places this project is honest about being a first version.

**Blocked detection is a similarity heuristic.** Keystone decides that a question belongs to a document the asker cannot read by comparing scores against a floor and a margin. On the demo documents no pair of values classified every labelled question correctly: true matches and non-matches overlap between about 0.35 and 0.47. The defaults are a choice, not a proof, and a different corpus needs re-tuning (section 5.4).

**The floor trades missed answers for false ones.** On the demo documents, 5 of 43 true matches (11.6%) score under the 0.40 floor and get "could not find an answer", and one near-topic question that no document answers scores above it. The blind spot is concrete: "How do I handle a request that is outside the signed scope?" scores 0.24 against the passage that answers it, because the document says "signed statement of work" and the question says "signed scope". A close variant, "What happens to work the client asks for that is not in the scope?", scores 0.3967, which is 0.0033 under the floor, and is also refused as no match. Embedding similarity follows wording more than meaning at the margins.

**A refusal reveals that a document exists, and which team holds it.** That is deliberate, because a refusal has to say who to ask. It is also an oracle: someone can probe whether a topic exists by asking about it. The refusal never contains a title, a passage or a score, but it is not silent.

**Access is by team only.** One team per employee, and a document is readable by whole teams. There are no per-person rules, no groups of teams and no per-document overrides. Teams are created from configuration, and cannot be renamed or removed in the app.

**Accounts are minimal.** Only one admin can be created through the app, the first-run account. There is no password reset or change, no way to disable or delete an account, and no single sign-on. Sessions are a bearer token held in the browser's session storage and expire after 12 hours. Five failed sign-ins in 15 minutes lock that address and employee number out for the rest of the window. That lock lives in memory.

**Documents are trusted.** The model is told to treat passage text as data, not instructions, but anyone who can upload a document can shape its answers. Only admins can upload. Uploads are `.md`, `.txt` and text-based `.pdf`: no scans, no OCR, no Word or spreadsheet files. A PDF is read as plain text, so layout and tables are lost. Files are limited to 5 MiB, and ingestion is synchronous, so the request returns when every passage is embedded. A large document on a rate-limited embeddings plan takes minutes in one request. Documents cannot be edited, only replaced.

**Answers can still be wrong.** Citation markers are checked to refer to passages that were supplied, but nothing checks that a sentence faithfully reflects its passage. Read the cited source when it matters. Answers are capped at 1,000 tokens and say so when they reach the cap.

**Embedding provider and model.** Built and measured against the MongoDB-hosted Voyage endpoint and `voyage-4-lite`. Swapping the provider or model means more than changing a key, as section 5.5 describes. Voyage's own endpoint is documented but untested here.

**Free-tier behaviour.** A free embeddings account allows only a few requests a minute. Keystone waits and says so on screen, serves one embedding call at a time and keeps a small in-memory cache of question vectors, but a burst of new questions from several people is slow by design. The free API host sleeps when idle and the first visit waits about a minute.

**Flood protection is in memory.** The per-address limit, the hourly and daily caps, the vector cache and the sign-in lock live in one process. A restart clears them, and a second instance would not share them. The address is read from `X-Forwarded-For`, which is only as trustworthy as the proxy in front of it.

**Search is an exact scan**, not an approximate index. That is correct, and it was only exercised at the size of the demo (76 passages). A large corpus would need an index, and nothing here has been measured at that scale.

**Browsers.** Driven end to end in Chromium, Firefox and WebKit at desktop size, and Chromium and WebKit at phone size: sign in, ask, refusal, grant, answer, opening a citation. Not tried on a real phone, in desktop Safari, or with a screen reader.

**The Compose web container runs the Vite development server.** That is right for trying it out and wrong for a production web server. The hosted setup in section 6 builds it properly.

**Some limits are constants, not settings.** A question is limited to 500 characters, uploads to 5 MiB, answers to 1,000 tokens, the whole deployment to 300 questions an hour, sessions to 12 hours, and sign-ins to 5 failures in 15 minutes. They are named constants in the code if you need to change them.

**Not audited.** The access design is tested, but this has not had an independent security review. Read the code and the tests before you trust it with documents that matter.

The demo company, its documents, the people in the screenshots and every number in them are fictional. They were invented to show the project working and are not part of the code.
