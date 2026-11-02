# CareerPilot AI

A local-first career-search workspace with public Greenhouse/Lever feeds and location-based public career-page discovery.

## Run locally

```bash
npm install
npm run dev
```

Install Go 1.26 or newer, Node.js, and [Ollama](https://ollama.com/). Pull a tool-capable local model before using the agent:

```bash
ollama pull qwen2.5:7b
npm run dev
```

The development command starts the Go platform API and the Vite console (web UI) together; the API listens on `127.0.0.1:8787`. To create a production console build, run `npm run build`.

The job-discovery agent uses the Ollama model configured by `OLLAMA_MODEL` (default `qwen2.5:7b`) and server URL configured by `OLLAMA_URL` (default `http://127.0.0.1:11434`). Use a model that supports tool calls. The candidate profile is sent to this model; keep `OLLAMA_URL` local if that data should not leave your machine.

## User settings and integrations

Open **Settings** in the sidebar to configure Gmail and LinkedIn OAuth or save other portal details. In Google Cloud, enable the Gmail API, configure the OAuth consent screen, and register the exact redirect URI shown in Settings (default: `http://localhost:8787/api/gmail/oauth/callback`). For LinkedIn, create an app, enable **Sign In with LinkedIn using OpenID Connect**, and register its displayed callback URI (default: `http://localhost:8787/api/linkedin/oauth/callback`). Gmail requests `gmail.readonly` to search job-related mail and `gmail.send` to send reviewed outreach; LinkedIn requests `openid profile email`. Reauthorize Gmail to grant the added send scope. Client secrets are encrypted before being written to SQLite and never returned to the frontend.

Gmail job-lead search returns a bounded set of thread content and sender/subject/date metadata to the local UI; messages and attachments are not sent to Ollama. Outreach drafts are editable and are not sent automatically. Each message requires an explicit final review and confirmation; send one recipient at a time. Applied-job closure notices are a separate monitoring notification sent to the connected Gmail account after three successful missing-listing checks; they never delete a job. LinkedIn OAuth is used only for the OIDC profile; it does not grant job-search or application API access. OAuth client configuration persists in the local database, but OAuth access tokens remain in memory, so reconnect after restarting the API.

The local database lives at `.data/careerpilot.db`; its encryption key is stored beside it with owner-only permissions. Do not separate the key from the database if you want to retain access to encrypted integration secrets and parsed resume text. Existing `.data/sources.json` board configuration is imported on first startup. `GMAIL_OAUTH_*` and `LINKEDIN_OAUTH_*` environment values are also imported into SQLite when no settings have yet been saved.

Profile and résumé settings are available under **My profile**. Uploading a text-based PDF or DOCX extracts its text and stores that text encrypted in SQLite. Scanned/image-only PDFs need OCR and are rejected with an explicit message. Discovered listings remain in memory during the session; jobs are saved in SQLite only after they are marked **Applied**, then their application statuses and monitoring history persist across restarts.

## Live job discovery

- A Stripe Greenhouse board is configured on first run as a working example.
- Add other public Greenhouse or Lever boards from **Job matches → Sources**. Enter the provider’s board ID (the path segment in its official jobs URL), not a full URL.
- **Area job search** searches public web results for jobs near the selected country, state/province, and city/area. It checks up to 20 public result pages per search, honors each site's `robots.txt`, and accepts structured `JobPosting` listings only from a company domain matching the hiring organization or a recognized ATS. No login, access-control bypass, or authenticated portal scraping is used.
- Configure a Brave Search API key in Settings for area search; it is encrypted in local SQLite. The search query includes your target role, skills, and selected location, but your resume text is not sent to Brave. Resume-based scoring runs locally. Results are live and are not saved as a company directory or permanent job index.
- Coverage is not exhaustive: search engines, public career pages, structured job data, location metadata, and crawl permissions vary. The app searches pages it can discover and verify; it cannot promise every company or vacancy in an area. Existing Greenhouse and Lever boards remain available under Job matches → Sources and can be scanned independently.
- Add Naukri or other portal records from **Settings → Other job portals**. Those records are configuration only: until a permitted provider adapter exists, the app does not search those sites or store portal passwords.
- **Find new jobs** fetches current public listings from each configured board, deduplicates results, and ranks them using keyword overlap with your profile’s skills, role, and preferred location.
- See the [job portal integration guide](./docs/PORTAL_INTEGRATION_GUIDE.md) for provider API versions, access requirements, adapter structure, and test expectations.
- Search/filter listings, save them, add jobs manually, and track application statuses. User profile, job sources, portal settings, OAuth client configuration, parsed resume text, and applied jobs persist in SQLite.
- Applied jobs from Greenhouse and Lever boards are rechecked every four hours. Three successful scans that omit a listing mark it for review; a failed board fetch does not count. A connected Gmail account receives a closure notice, and you must confirm removal in **Applications** or choose **Keep**. Jobs without a supported public feed are stored as applied but cannot be monitored for closure.
- **Agent activity → Agentic workflow** runs a bounded job-search agent with a live step view and trace log: plan queries, search (including Greenhouse, Lever, Ashby and SmartRecruiters hosts), verify pages and postings (robots.txt honored, ATS postings read from their public JSON APIs), score against your resume locally and filter weak matches, then optionally summarize the best matches with your local Ollama model (job summaries and your title/skills only; never resume text). Pages that return 401/403 are skipped and reported, never logged into. Model, Ollama URL, search rounds, target matches, page budget, minimum score and the model review toggle are configurable under **Agent settings** and saved in SQLite. One run is active at a time and can be stopped.
- Open a job and use **Check readiness** to see whether profile details, a parsed résumé, and a valid job URL are ready for preparation.
- The current application workflow is preparation-only. No application is submitted; future portal submissions must require explicit user confirmation. Gmail outreach can be prepared from job-related messages, but each email must be reviewed and explicitly confirmed before sending.
- Run the Go tests (platform and intelligence) with `npm run test:api`; run the console type-check and production build with `npm run build`.

## Repository layout

| Directory | Role |
| --- | --- |
| `console/` | React + Vite web UI. Talks to the platform through the `/api` contract. |
| `platform/` | Go API service: HTTP API, SQLite storage, OAuth and job-board integrations, composition root. |
| `intelligence/` | AI layer: agents, tools, LLM providers, workflows, run management, browser automation. The home for upcoming neural / deep-learning work. |
| `contracts/` | Shared data types (`contracts/domain`) that both `platform` and `intelligence` depend on. |
| `cortex/` | Python AI service (FastAPI): agent orchestrator, embeddings, skill-gap analysis. Called by the platform over HTTP at `/api/ai/*`. |

### Cortex (Python AI service)

Setup once with `npm run setup:ai` (needs Python 3.10–3.13, e.g. `brew install python@3.12`; it also downloads Chromium for Playwright). `npm run dev` then starts the service on `127.0.0.1:8890`. Run its tests with `npm run test:ai`.

- The platform attaches your saved profile and résumé server-side; the browser never sends résumé text.
- **AI analysis** ranks jobs with embeddings and skill-gap analysis. **Processing workflow** shows every step, timing and trace of any run as a live graph.
- **Application copilot** is a LangGraph workflow: scrape the job page (Playwright, with an HTTP fallback) → index your résumé in ChromaDB → match → tailor bullets → fact-check (loops back to the tailor if a bullet invents a skill or number) → cover note and outreach → **pause for your approval**. Weak fits (below 35%) stop before drafting unless you choose "draft anyway".
- **Nothing is ever sent.** Approving only records your decision; you copy the text yourself. The MCP server has no approve or send tool either.
- **Multi-agent team:** after the match, a Research agent (live web search) and a Strategy agent (positioning and gap plan) run in parallel, the Tailor and Fact-check agents loop on the bullets, the Outreach agent writes, a Critic agent reviews the text, and you approve. All steps and agents show in the live graph.
- **Key expiry:** set `CORTEX_KEY_EXPIRES=YYYY-MM-DD` (from the OpenAI dashboard). The Application copilot page warns 14 days before it expires. Keys cannot be rotated automatically; create a new one and update `cortex/.env`.
- **Research agent:** with an OpenAI key, a step searches the live web for the company and feeds that context to the cover note and outreach message. Only the company name and role are sent, never your résumé, and the results are treated as untrusted text. It is skipped with other models.
- Put keys in `cortex/.env` (gitignored, loaded by `npm run dev:ai`), for example `OPENAI_API_KEY=...` and optionally `CORTEX_LLM_MODEL=gpt-4o-mini`. Never paste keys into source files or chats.
- Without an LLM, bullets are verbatim résumé passages and the UI warns about it. Run state is in memory and is lost when the service restarts.
- The scraper only reads public `http(s)` pages on ports 80/443, rejects private and loopback addresses (also on redirects and sub-requests), honours `robots.txt` and caps page size.

| Env var | Purpose |
| --- | --- |
| `CORTEX_URL` | Platform → service address (default `http://127.0.0.1:8890`). |
| `CORTEX_LLM` | `ollama`, `openai`, `anthropic` or `none`. Defaults to `openai` when `OPENAI_API_KEY` is set, otherwise `ollama`. Hosted models send résumé text off this machine; the UI warns. |
| `CORTEX_LLM_MODEL`, `OLLAMA_URL`, `OLLAMA_MODEL`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY` | Model choice and credentials. On an 8 GB machine use `qwen2.5:1.5b` (a full draft takes about 80 s). |
| `OLLAMA_KEEP_ALIVE` (default `30m`), `OLLAMA_NUM_CTX` (default `2560`) | Keep the model loaded between runs and cap its context, to avoid cold loads and swapping. |
| `CORTEX_KEY_EXPIRES` | Expiry date of the hosted-model key, for the warning banner. |
| `CORTEX_EMBEDDER` | `auto` (default): the all-MiniLM-L6-v2 neural network on onnxruntime (bundled with chromadb, about 90 MB downloaded once), falling back to `hashing` (lexical) if it cannot load. `onnx` and `hashing` force one; `sentence-transformers` uses torch after `pip install -r cortex/requirements-ml.txt`. `CORTEX_MODEL` picks the model. |
| `CORTEX_SCRAPER` | `http` disables the headless browser. |
| `CORTEX_DATA_DIR` | Persist the ChromaDB index; in-memory when unset. |
| `PHOENIX_COLLECTOR_ENDPOINT` | Enable Phoenix tracing (after `pip install -r cortex/requirements-observability.txt`). Loopback only unless `CORTEX_TRACING_ALLOW_REMOTE=1`; spans contain résumé and job text. |
| `CAREERPILOT_API_URL` | Where the MCP server finds the platform (default `http://127.0.0.1:8787`). |

MCP tools (parse a job, match, search résumé, draft an application, read a draft, update the profile): `cd cortex && .venv/bin/python -m cortex.mcp_server` (stdio).

Endpoints: `GET /api/ai/status`, `GET /api/ai/pipeline`, `GET|POST /api/ai/analyses`, `GET /api/ai/analyses/{id}`, `POST /api/ai/analyses/{id}/cancel`, `GET /api/ai/application-pipeline`, `GET|POST /api/ai/applications`, `GET /api/ai/applications/{id}`, `POST /api/ai/applications/{id}/decision`, `POST /api/ai/applications/{id}/cancel`, `POST /api/ai/portals/search`, `GET|POST|DELETE /api/ai/portals/session`, `POST /api/ai/applications/{id}/apply`, `POST /api/sources-catalog`.

**Console layout:** Overview, Applications, **Jobs** (matches, area search, company portals), **Agents** (application copilot, job analysis, workflow, job scout; a strip shows the profile, résumé and model it will use) and **Admin** (profile and résumé, sources and integrations, AI and sign-ins).

**More jobs, automatically:** *Jobs → Company portals → Find engineering jobs* connects 25 engineering employers' public Greenhouse/Lever boards (each is probed first, so dead ones are skipped) and scans them; the console also rescans every 30 minutes while the agent is running. Results are ranked by your profile.

**Apply (after approval):** once you approve a draft, the *Apply agent* step opens the job's application form in a visible browser (using your saved sign-in) and pre-fills your name, email, location and cover note. It never presses Submit; you review, attach your résumé, answer any questions and submit yourself. It needs the job's page address.

**Company portals:** paste a careers page and Cortex lists and ranks the openings. If the site answers 401/403, the console offers *Sign in with Google*, which opens an ordinary visible Chromium window; you sign in yourself and close it. Only the browser session is kept, in `cortex/data/browser-profile` (gitignored) and reused by later reads; Admin → AI & sign-ins deletes it. CareerPilot never sees your Google password. Pages are still read only if robots.txt allows, and only on public hosts. Some sites will refuse sign-in or automated reading regardless.

Dependency rule (enforced by `platform/internal/architecture`): `contracts` imports nothing in this repo, `intelligence` may import only `contracts`, and `platform` may import both. The platform supplies data and side effects (storage, web search, HTTP) to the agents through interfaces and shared types, so models can change without touching the API.

## Platform architecture

The platform is a Go modular monolith under `platform/`:

- `internal/integrations/greenhouse` and `internal/integrations/lever` are independent adapters, each separating client, job retrieval, response mapping, and provider response types.
- `internal/integrations/jobs` owns shared job-provider contracts, the provider registry, HTTP helpers, and the normalized job mapper.
- `internal/integrations/linkedin` follows the client/OAuth/jobs/profiles/mapper layout. LinkedIn OAuth uses state and the OIDC userinfo endpoint for basic profile details only. Job search stays disabled until approved product access and an appropriate adapter are configured.
- `internal/integrations/gmail` implements Google OAuth with state/PKCE, an in-memory token source, bounded thread search, explicit-confirmation email sending, and applied-job closure notices through Google's official Gmail API client.
- `internal/integrations/webjobs` searches public career pages, respects `robots.txt`, verifies employer domains or recognized ATS hosts, and extracts structured job postings.
- `internal/storage` persists local user configuration in SQLite and encrypts OAuth client secrets and parsed resume text with AES-GCM.
- `internal/services/discovery` composes provider clients into the discovery use case.
- `intelligence/workflow` defines explicit, observable discovery stages and the discovery request/output types.
- `intelligence/tools` registers tools with risk classifications; external actions require explicit approval. The first Genkit tool is read-only job discovery and is limited to one execution per request.
- `intelligence/agents` implements the first Genkit agent using Ollama; only the read-only public-job discovery tool is exposed to its model.
- `intelligence/runs` runs and tracks the area-search agent, with an optional Ollama review of results.
- `intelligence/llm` defines a provider-neutral generation interface and an OpenAI-compatible adapter.
- `intelligence/browser` defines the browser automation boundary and requires timestamped user approval before submission. A Playwright driver is not implemented yet.
- `internal/api` owns the local HTTP API, profile/settings/portal configuration, resume parsing, and SQLite-backed source configuration.
- `internal/app` is the composition root that wires provider adapters, discovery, tools, and HTTP API.
- `cmd/careerpilot` owns process startup and graceful shutdown.

The React console uses the `/api` contract through the Vite development proxy.

The integration design uses **Adapter** (provider APIs implement one job-provider interface), **Registry** (provider lookup by type), **Mapper** (provider payload to application model), and **Composition Root** (`internal/app`). To add another job source, add a provider package and register it in the composition root instead of changing discovery workflows or HTTP routes.

## Boundaries

The live job-board integrations include Greenhouse and Lever public APIs plus bounded public career-page discovery through Brave Search. Other boards require permitted official feeds/adapters. Area search is not exhaustive and only accepts structured postings from matching employer domains or recognized ATS hosts; it honors robots.txt and does not use authenticated portal access. Gmail credentials are used only with Gmail. Fit scoring is a transparent keyword heuristic, not an LLM assessment. Discovery runs on demand; only already-applied jobs are monitored in the background. Résumés are text-extracted, not semantically interpreted. Application readiness checks are implemented, but application preparation/submission through external portals is not; no Playwright driver exists. There is no user authentication or multi-user hosting, so do not treat this development server as a production deployment.


## Deployment (Railway)

Pushes to `main` deploy automatically through Railway's GitHub integration (CI runs separately in GitHub Actions). One Railway project, two services:

| Service | Root dir | Contains | Storage |
| --- | --- | --- | --- |
| `careerpilot-ai` (public) | `/` | Go API + built React console, one container | SQLite in `/data` volume |
| `careerpilot-cortex` (private) | `/cortex` | Python AI service (FastAPI, ChromaDB, Playwright) | ChromaDB in `/data` volume |

The browser talks only to `careerpilot-ai`. It serves the UI and `/api/*`, and calls cortex over Railway's private network via `CORTEX_URL=http://careerpilot-cortex.railway.internal:8890`. Cortex has no public domain. Each service has its own `railway.json` with watch patterns, so a change only rebuilds the service it affects.

SQLite and ChromaDB live on single-writer volumes: keep each service at one replica. Set `CORTEX_LLM` plus the matching API key on cortex for real model output (default is template mode).
