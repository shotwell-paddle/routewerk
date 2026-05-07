# Competition Tracking — Implementation Handoff (v3, post-audit)

This document is the implementation plan for adding competition tracking and management to Routewerk, and the start of Routewerk's migration to a SPA-first frontend. First production target: bouldering league at LEF/Mosaic.

Read `CLAUDE.md` first. This plan adds new conventions on top of those.

Phase 0 (SvelteKit + embed pipeline) shipped in PRs [#30](https://github.com/shotwell-paddle/routewerk/pull/30) and [#31](https://github.com/shotwell-paddle/routewerk/pull/31). Phase 1 work begins below.

---

## What changed from v1

This is a substantial revision. Major changes:

- **Frontend goes SPA-first.** SvelteKit, served as embedded static assets from the Go binary. Existing HTMX pages remain operational; new comp UI is SvelteKit. Other modules migrate when touched.
- **Score recomputation is synchronous in-transaction.** No worker, no materialized scores table. Computed via SQL window functions on read, with short-TTL caching.
- **Single action endpoint with idempotency keys.** Climber actions batch and replay cleanly. No per-button endpoints.
- **Walk-ins removed from scope.** Accounts mandatory. Magic-link login + one-tap registration handles the UX.
- **SSE for live updates.** Replaces HTMX polling for the leaderboard.
- **Scorer interface revised.** `Rank()` operates on raw attempts so IFSC-style count-back tiebreaks work correctly.

## What changed from v3

- **Comp + event + category CRUD now requires `head_setter+`** (was `gym_manager+`). Head setters already own problem set edits; lowering comp-shell CRUD to the same gate rounds out the comp-setup story so head setters can build comps end-to-end without a manager hand-off. Affected paths: `POST /locations/{id}/competitions`, `PATCH /competitions/{id}`, `POST /competitions/{id}/events`, `PATCH /events/{id}`, `POST /competitions/{id}/categories`. Registration management gates are unchanged.

## What changed from v2 (post-audit reconciliation, 2026-05-05)

After auditing the actual repo, these adjustments apply throughout this doc:

- **Tables: `gyms` → `locations`.** Routewerk's hierarchy is `organizations → locations → walls → routes`. There is no `gyms` table; references in this doc use `locations` / `location_id`.
- **No separate `cmd/worker/` binary.** Background work runs in-process via `internal/jobs/queue.go` (Postgres-backed queue, SKIP LOCKED). The comp module needs no jobs; the existing queue is left alone.
- **Migration numbering.** Starts at `000032_competitions`, padded to 6 digits.
- **Role syntax.** Roles are exact strings: `climber`, `setter`, `head_setter`, `gym_manager`, `org_admin`. The `gym_manager+` shorthand maps to `authz.RequireLocationRole("gym_manager", "org_admin")` (or whichever middleware helper applies). `org_admin` implicitly satisfies any lower-rank requirement.
- **`format` collapses into `aggregation` jsonb.** Originally a four-value enum (`single|series|series_drop|season_finals`) AND `aggregation` jsonb — redundant. Schema now uses `format text` with two values (`single|series`) and lets `aggregation jsonb` carry everything that varies (`{ "method": "sum"|"sum_drop_n"|"weighted_finals"|"best_n", "drop": 1, "weights": [1,1,1,2], "finals_event_id": "..." }`). Adding new aggregation shapes later is config, not migration.
- **Per-event scoring rule override.** `competition_events` gets `scoring_rule_override text NULL` and `scoring_config_override jsonb NULL` so a single series can mix event types (e.g. boulder night + speed night under the same comp).
- **Public leaderboard default.** `competitions.leaderboard_visibility text NOT NULL DEFAULT 'public'` (also accepts `members`, `registrants`).
- **Magic link is a real Phase 1 work item.** No transactional auth flow exists today (SMTP is wired but unused for that). Build properly: `magic_link_tokens` table (hashed token + expiry + single-use), `/auth/magic/request` and `/auth/magic/verify` endpoints, rate limit per-email and per-IP, mailed via existing SMTP service. Magic link is *additive* to password login, not a replacement.

---

## Goals

- Run multi-format climbing competitions: single events, multi-week series with optional drop-lowest, season + finals.
- Self-scoring on phone with optimistic UI and real-time leaderboard updates.
- Flexible scoring via a registered scorer interface.
- Live leaderboard suitable for both phone and large-screen (gym TV) display.
- Staff override and full audit trail.
- Establish the SPA-first frontend pattern for Routewerk going forward.

## Non-goals (for v1)

- Native mobile app (Flutter API stays in scope; app changes don't).
- Service Worker / true offline. Optimistic UI + retry-on-reconnect is enough for gym wifi.
- Walk-in registration without an account.
- Payment processing.
- Migrating existing HTMX pages to SvelteKit. Comp module only for now.

---

## Stack decisions

| Area | Choice | Why |
|---|---|---|
| Backend | Go 1.24, chi, pgx/v5, Postgres 16 | Unchanged. No reason to switch. |
| Frontend (new) | SvelteKit + TypeScript, static adapter | Smaller bundles, faster solo iteration, runes are a strong fit for live UI, no overlap with SendIt v2's React mental model. |
| Frontend (legacy) | html/template + HTMX continues running | Not migrating other modules in this work. |
| Hosting | Single Fly app. SvelteKit built to static, embedded into Go binary via `embed`, served alongside API. | Single deploy, no CORS, cookie auth keeps working. |
| Auth (web/SPA) | Cookie sessions (existing) | Same origin, CSRF via existing double-submit pattern. |
| Auth (mobile/API) | JWT (existing) | Future Flutter app. |
| API contract | REST + OpenAPI spec, codegen for Go server interfaces and TypeScript client | Single source of truth, mobile-app friendly, no TS↔Go coupling. |
| Real-time | Server-Sent Events | One-way push, native browser EventSource, ~50 lines of Go. |
| Score computation | Synchronous in same transaction as write; SQL `RANK() OVER` for ranking | Sub-millisecond at comp scale. No worker, no race conditions. |
| Offline | Not in v1 | Optimistic UI + retry-on-reconnect handles the realistic failure mode (brief signal loss). |

---

## Architecture summary

```
Browser (SvelteKit SPA)
  ├── Climber view — optimistic mutations, SSE leaderboard
  ├── Big-screen view — full-page leaderboard, SSE driven
  └── Staff dashboard — admin CRUD, verification, override
        │
        │  HTTPS (same origin)
        ▼
Go binary on Fly.io
  ├── Embedded SPA static assets at /
  ├── REST API at /api/v1/*
  ├── SSE endpoints at /api/v1/*/stream
  ├── Existing HTMX routes at /login, /dashboard, /routes, etc.
  └── Postgres 16

Domain:
Competition (location, slug, format=single|series, aggregation jsonb,
             scoring_rule, scoring_config, leaderboard_visibility)
  ├── Events (sequence, weight, time window, optional scoring_rule_override)
  │     └── Problems (label, optional route_id, points/zone_points)
  ├── Categories (rules: age/gender/etc.)
  └── Registrations (user_id required, category, bib)
        └── Attempts (one row per problem; full history in attempt_log)
```

Scores are not stored. The leaderboard is computed on demand by SQL query over `competition_attempts` joined with `competition_problems`, partitioned by category, ranked by the active scorer's logic. Cache the rendered API response 1–2 seconds with invalidation on write.

---

## Database migrations

Three migrations starting at `000032_*`.

### Migration A — `000032_competitions.up.sql`

```sql
CREATE TABLE competitions (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id     uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
  name            text NOT NULL,
  slug            text NOT NULL,
  -- format = single | series. Aggregation specifics live in `aggregation`.
  format          text NOT NULL CHECK (format IN ('single','series')),
  -- aggregation jsonb shape (Postgres doesn't constrain; validated in app code):
  --   { "method": "sum"|"sum_drop_n"|"weighted_finals"|"best_n",
  --     "drop": 1,                        // for sum_drop_n / best_n
  --     "weights": [1,1,1,2],             // for weighted_finals
  --     "finals_event_id": "<uuid>" }     // for weighted_finals
  aggregation     jsonb NOT NULL DEFAULT '{}'::jsonb,
  -- Default scorer for the comp; events may override (see competition_events).
  scoring_rule    text NOT NULL,
  scoring_config  jsonb NOT NULL DEFAULT '{}'::jsonb,
  status          text NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','open','live','closed','archived')),
  leaderboard_visibility text NOT NULL DEFAULT 'public'
                    CHECK (leaderboard_visibility IN ('public','members','registrants')),
  starts_at              timestamptz NOT NULL,
  ends_at                timestamptz NOT NULL,
  registration_opens_at  timestamptz,
  registration_closes_at timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (location_id, slug)
);

CREATE TABLE competition_events (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  competition_id  uuid NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  name            text NOT NULL,
  sequence        int  NOT NULL,
  starts_at       timestamptz NOT NULL,
  ends_at         timestamptz NOT NULL,
  weight          numeric NOT NULL DEFAULT 1.0,
  -- Optional per-event override of the comp-level scorer. Lets a single
  -- series mix event types (e.g. boulder night + speed night).
  scoring_rule_override   text,
  scoring_config_override jsonb,
  UNIQUE (competition_id, sequence)
);

CREATE TABLE competition_categories (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  competition_id  uuid NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  name            text NOT NULL,
  sort_order      int  NOT NULL DEFAULT 0,
  rules           jsonb NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE (competition_id, name)
);

CREATE TABLE competition_problems (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id        uuid NOT NULL REFERENCES competition_events(id) ON DELETE CASCADE,
  route_id        uuid REFERENCES routes(id) ON DELETE SET NULL,
  label           text NOT NULL,
  points          numeric,
  zone_points     numeric,
  grade           text,
  color           text,
  sort_order      int NOT NULL DEFAULT 0,
  UNIQUE (event_id, label)
);

CREATE INDEX competitions_location_status_idx ON competitions(location_id, status);
CREATE INDEX competition_events_comp_idx ON competition_events(competition_id, sequence);
CREATE INDEX competition_problems_event_idx ON competition_problems(event_id, sort_order);
```

### Migration B — `000033_competition_registrations.up.sql`

```sql
CREATE TABLE competition_registrations (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  competition_id  uuid NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  category_id     uuid NOT NULL REFERENCES competition_categories(id) ON DELETE RESTRICT,
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  display_name    text NOT NULL,                  -- snapshot at registration
  bib_number      int,
  waiver_signed_at timestamptz,
  paid_at         timestamptz,
  withdrawn_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (competition_id, user_id)
);

-- Bib uniqueness only among active registrations; freed when withdrawn.
CREATE UNIQUE INDEX competition_reg_bib_active_idx
  ON competition_registrations(competition_id, bib_number)
  WHERE withdrawn_at IS NULL AND bib_number IS NOT NULL;

CREATE INDEX competition_reg_comp_idx ON competition_registrations(competition_id);
```

### Migration C — `000034_competition_attempts.up.sql`

```sql
CREATE TABLE competition_attempts (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  registration_id  uuid NOT NULL REFERENCES competition_registrations(id) ON DELETE CASCADE,
  problem_id       uuid NOT NULL REFERENCES competition_problems(id) ON DELETE CASCADE,
  attempts         int  NOT NULL DEFAULT 0,
  zone_attempts    int,
  zone_reached     bool NOT NULL DEFAULT false,
  top_reached      bool NOT NULL DEFAULT false,
  notes            text,
  logged_at        timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  verified_by      uuid REFERENCES users(id),
  verified_at      timestamptz,
  UNIQUE (registration_id, problem_id)
);

CREATE INDEX competition_attempts_problem_idx ON competition_attempts(problem_id);

CREATE TABLE competition_attempt_log (
  id               bigserial PRIMARY KEY,
  attempt_id       uuid NOT NULL REFERENCES competition_attempts(id) ON DELETE CASCADE,
  actor_user_id    uuid REFERENCES users(id),
  action           text NOT NULL,
  before           jsonb,
  after            jsonb,
  idempotency_key  uuid,
  at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX competition_attempt_log_attempt_idx
  ON competition_attempt_log(attempt_id, at DESC);

-- Idempotency dedupe across the whole competition module.
CREATE UNIQUE INDEX competition_attempt_log_idem_idx
  ON competition_attempt_log(idempotency_key)
  WHERE idempotency_key IS NOT NULL;
```

No `competition_scores` table. Score computation is at read time via SQL.

Table names verified against the existing schema: `locations`, `routes`, and `users` are the actual names. The `gyms` reference in v2 was wrong (corrected throughout).

---

## Code structure

### Backend (Go)

```
cmd/api/                          # unchanged
cmd/admin/                        # unchanged
internal/
  model/
    competition.go
  repository/
    competition.go                # comps + events + categories + problems
    competition_registration.go
    competition_attempt.go        # attempts + log writes
    competition_leaderboard.go    # the read-time ranking query
  service/
    competition/
      scorer.go                   # Scorer interface + registry
      topzone.go                  # IFSC-style impl
      fixed.go                    # fixed-points impl
      decay.go                    # redpoint-decay impl
      aggregate.go                # series/season aggregation
      scorer_test.go
  handler/
    competition.go                # API CRUD
    competition_action.go         # the unified action endpoint
    competition_leaderboard.go    # read endpoints + SSE stream
  router/
    (register new routes — see below)
internal/sse/                     # new package: SSE hub for fan-out
  hub.go
  hub_test.go
```

No `cmd/worker/score_recompute.go`. Synchronous computation removes the worker entirely from the comp module. The existing worker binary stays for whatever else it does.

### Frontend (SvelteKit)

```
web/spa/
  package.json
  svelte.config.js                # static adapter, output to ./build
  vite.config.ts
  src/
    routes/
      +layout.svelte              # auth wrapper, global stores
      +page.svelte                # /  → redirect to active comp or list
      comp/
        [slug]/
          +page.svelte            # climber scorecard
          +page.ts                # data load
          p/[label]/+page.svelte  # problem detail (full-screen tap targets)
          leaderboard/+page.svelte
          big-screen/+page.svelte # TV-optimized leaderboard
      staff/
        comp/
          +page.svelte            # comp list
          new/+page.svelte
          [slug]/
            +page.svelte          # admin dashboard
            problems/+page.svelte
            registrations/+page.svelte
            verify/+page.svelte
    lib/
      api/
        client.ts                 # generated from OpenAPI
        actions.ts                # action queue + retry logic
      stores/
        leaderboard.ts            # SSE-driven Svelte store
        scorecard.ts              # local optimistic state
      components/
        ScoreChip.svelte
        ProblemCard.svelte
        LeaderboardRow.svelte
        ActionButton.svelte
    app.html
  static/
```

The SvelteKit build output (`web/spa/build/`) is embedded into the Go binary via `//go:embed` in a new file, e.g. `internal/spa/embed.go`. The Go server checks for `/api/*` first, then the existing HTMX routes, then falls through to serving the SPA's `index.html` for any unmatched route (SPA client-side routing handles the rest).

---

## Action API

Single endpoint replaces per-button POSTs. Climber actions queue locally if the network blips and replay automatically.

```
POST /api/v1/competitions/{id}/actions
  Body: { actions: [
    { idempotency_key: uuid, type: 'increment'|'zone'|'top'|'undo'|'reset',
      problem_id: uuid, client_timestamp: ISO8601 }
    ...
  ]}
  Response: { applied: [...], rejected: [...], state: { ... current attempts ... } }
```

Server behavior:

1. For each action, look up by `idempotency_key` in `competition_attempt_log`. If found, return the existing result — already applied.
2. Validate: registration matches authenticated user, problem belongs to active event, event hasn't ended.
3. Apply action to `competition_attempts` (upsert).
4. Append to `competition_attempt_log` with idempotency key.
5. Compute current attempt state, return.
6. Publish event to SSE hub for the affected `(competition_id, category_id)`.

Action types:
- `increment` — add an attempt
- `zone` — mark zone reached (only meaningful if scorer uses zones)
- `top` — mark sent
- `undo` — pop most recent action via attempt_log
- `reset` — set everything for that problem back to 0 (staff only, or climber within grace window)

Client behavior (SvelteKit):

1. Tap is applied to local store immediately (optimistic).
2. Action enqueued in a TanStack Query mutation queue (or svelte-query equivalent).
3. Mutation fires; on success, server response reconciles local state.
4. On network failure, mutation retries with exponential backoff.
5. Idempotency key generated client-side per action ensures no duplicates.

---

## Scoring engine

### Interface

```go
// internal/service/competition/scorer.go
package competition

import (
    "encoding/json"

    "github.com/google/uuid"
)

type Attempt struct {
    RegistrationID uuid.UUID
    ProblemID      uuid.UUID
    Attempts       int
    ZoneAttempts   *int
    ZoneReached    bool
    TopReached     bool
}

type Problem struct {
    ID         uuid.UUID
    Label      string
    Points     *float64
    ZonePoints *float64
    SortOrder  int
}

type ClimberScore struct {
    RegistrationID uuid.UUID
    Points         float64
    Tops, Zones    int
    AttemptsToTop  int
    AttemptsToZone int
    PerProblem     []ProblemScore  // for tiebreak count-back
    Detail         map[string]any
}

type ProblemScore struct {
    ProblemID    uuid.UUID
    Attempts     int
    ZoneAttempts *int
    Points       float64
    TopReached   bool
    ZoneReached  bool
}

type RankedScore struct {
    ClimberScore
    Rank int
}

type Scorer interface {
    Name() string

    // Score one climber from their attempts.
    Score(attempts []Attempt, problems []Problem, cfg json.RawMessage) (ClimberScore, error)

    // Rank a category. Operates on full per-climber scores including PerProblem
    // detail so count-back tiebreaks (compare best problem, then second-best,
    // etc.) work correctly.
    Rank(scores []ClimberScore, cfg json.RawMessage) []RankedScore
}

var registry = map[string]Scorer{}

func Register(s Scorer) { registry[s.Name()] = s }

func Get(name string) (Scorer, bool) {
    s, ok := registry[name]
    return s, ok
}
```

Each rule registers itself in `init()`. Per CLAUDE.md, scorers are pure functions and the right place for rigorous test coverage. Table-driven tests required for each.

### Rules to implement

1. **`top_zone`** — IFSC-style. Tops, zones, attempts-to-top, attempts-to-zone, count-back. No config needed.
2. **`fixed`** — Sum of `competition_problems.points` for sent problems. Optional `flash_bonus` config.
3. **`decay`** — `score = base / (1 + rate * (attempts - 1))` per sent problem, summed. Config: `{ "base_points": 1000, "decay_rate": 0.1, "flash_bonus": 0 }`.

---

## SSE hub

`internal/sse/hub.go` — generic SSE fan-out, used initially by leaderboard but designed to be reused.

```go
type Hub struct {
    mu          sync.RWMutex
    subscribers map[string]map[chan []byte]struct{}  // topic → set of subscriber channels
}

func (h *Hub) Subscribe(topic string) (<-chan []byte, func())
func (h *Hub) Publish(topic string, msg []byte)
```

Topics: `comp:{id}:cat:{id}` for category-level updates, `comp:{id}` for comp-level.

API endpoint:

```
GET /api/v1/competitions/{id}/leaderboard/stream?category={id}
  Content-Type: text/event-stream
  Sends: snapshot on connect, then deltas on every relevant write
```

Handler subscribes to the hub topic, writes SSE frames to the response, unsubscribes on client disconnect. In-process only for v1 — fine for single-region Fly deploy. If you ever scale horizontally, swap the hub for Postgres LISTEN/NOTIFY behind the same interface.

---

## Routes

API:

```
# Comp CRUD
POST   /api/v1/locations/{location}/competitions   head_setter+
GET    /api/v1/locations/{location}/competitions
GET    /api/v1/competitions/{id}
PATCH  /api/v1/competitions/{id}                   head_setter+

# Comp setup
POST   /api/v1/competitions/{id}/events            head_setter+
PATCH  /api/v1/events/{id}                         head_setter+
POST   /api/v1/competitions/{id}/categories        head_setter+
POST   /api/v1/events/{id}/problems                head_setter+
PATCH  /api/v1/problems/{id}                       head_setter+
POST   /api/v1/events/{id}/problems/import         head_setter+   # CSV

# Registration
POST   /api/v1/competitions/{id}/registrations     authenticated user (self) | staff (other)
GET    /api/v1/competitions/{id}/registrations     authenticated user (own) | staff (all)
DELETE /api/v1/registrations/{id}                  staff or self

# Action
POST   /api/v1/competitions/{id}/actions           authenticated, registered

# Verification
POST   /api/v1/attempts/{id}/verify                setter+
POST   /api/v1/attempts/{id}/override              setter+

# Leaderboard
GET    /api/v1/competitions/{id}/leaderboard       per-comp visibility setting
GET    /api/v1/events/{id}/leaderboard
GET    /api/v1/competitions/{id}/leaderboard/stream  SSE
```

Web (SPA serves all of these client-side; server only needs to send `index.html`):

```
/comp/{slug}                    → climber scorecard (active event)
/comp/{slug}/p/{label}          → problem detail
/comp/{slug}/leaderboard        → mobile-friendly leaderboard
/comp/{slug}/big-screen         → TV-optimized leaderboard
/staff/comp                     → comp list
/staff/comp/{slug}              → admin dashboard
/staff/comp/{slug}/problems     → problem set editor
/staff/comp/{slug}/registrations → registration list
/staff/comp/{slug}/verify       → verification queue
```

---

## Authz

- Comp + event + category CRUD → `head_setter+`
- Problem set edits → `head_setter+`
- Action endpoint → authenticated, registered, registration's user matches caller, event not ended
- Verify / override → `setter+`, logged with `action='verify'` or `action='override'`
- Leaderboard → enforced per `competitions.leaderboard_visibility` (`public` | `members` | `registrants`). Default is `public` per the LEF/Mosaic league plan; staff can lock down per-comp.

---

## Self-score UX

Phone-first. SvelteKit handles optimistic UI; the action queue handles transient failures.

1. Climber logs in (magic link recommended for league night) → SPA loads at `/`.
2. SPA detects active comp registration for user → redirects to `/comp/{slug}`.
3. Scorecard shows all problems for the active event with current per-problem state.
4. Tap a problem → `/comp/{slug}/p/{label}` (client-side route, no full reload).
5. Three big tap targets: **+1 Attempt**, **Got Zone**, **Sent It**. **Undo** below.
6. Each tap:
   - Applies optimistically to local Svelte store (instant feedback, ~16ms)
   - Generates idempotency key
   - Enqueues action in mutation queue
   - Mutation fires; on success, reconciles state from server response
   - On network failure, retries with exponential backoff in background
7. After "Sent It," scroll to next unsent problem.
8. Rank chip in corner subscribes to SSE; updates live as their score changes.

Guardrails:
- Server rejects writes after event `ends_at`. Client UI greys out buttons and shows "Event closed."
- Per-registration rate limit (60 actions/minute) on top of existing IP rate limiting.
- Optional witness mode (config flag): "Sent It" prompts for confirmation by another bib number.

---

## Big-screen leaderboard

Separate route, designed for a TV at the gym. SPA route `/comp/{slug}/big-screen`.

- Full-page leaderboard, large type, no chrome.
- Subscribed to SSE for instant updates.
- Animation when a rank changes (smooth row reorder).
- Rotates through categories every 20 seconds if multiple categories.
- Optional QR code in corner for climbers to register/log in on their phone.

---

## OpenAPI + codegen

Spec lives at `api/openapi.yaml`. Build pipeline:

- `make api-gen` → runs `oapi-codegen` to generate Go server interfaces in `internal/handler/gen/`
- `cd web/spa && npm run api-gen` → runs `openapi-typescript` to generate TS types in `src/lib/api/types.ts`
- API client wrapper in `src/lib/api/client.ts` uses these types

Both generators run in CI to verify spec matches implementation.

This means the OpenAPI spec is the single source of truth. Adding an endpoint = update spec → regenerate both sides → implement.

---

## Phasing

### Phase 0 — Foundation ✅ SHIPPED

PRs [#30](https://github.com/shotwell-paddle/routewerk/pull/30) (scaffold) and [#31](https://github.com/shotwell-paddle/routewerk/pull/31) (route fix). Live at `routewerk-dev.fly.dev/spa-test/`.

### Phase 1 — Comp launch (the goal)

Broken into sub-PRs so each is reviewable in isolation. Order matters: 1a is the foundation for everything else; 1b and 1c are independent and can land in either order; 1d is parallelizable. The UI sub-phases (1g–1i) need 1e/1f wired first.

- **1a — Schema + models + repos.** Migrations 000032–000034, `internal/model/competition.go`, four repos (competition, registration, attempt, leaderboard read query stub). No business logic, no handlers, no UI.
- **1b — Scorer interface + first scorer.** `internal/service/competition/{scorer,topzone,registry}.go` with table-driven tests. No DB; pure functions.
- **1c — SSE hub.** `internal/sse/{hub,hub_test}.go`. Generic fan-out, no comp specifics.
- **1d — Magic link auth.** `magic_link_tokens` table, `/auth/magic/request` and `/auth/magic/verify` endpoints, email template, rate limit, tests. Independent of comp work — could ship first if there's appetite.
- **1e — OpenAPI spec + codegen wiring.** Populate `api/openapi.yaml` with comp endpoints; wire `oapi-codegen` (Go server interfaces) and `openapi-typescript` (TS client types) into `make api-gen` and CI.
- **1f — API handlers.** Implement the codegen interfaces. Single-action endpoint with idempotency, leaderboard read endpoint + SSE stream. Tests via `httptest`.
- **1g — Climber UI.** SvelteKit pages: `/comp/{slug}` scorecard with optimistic UI + action queue, `/comp/{slug}/p/{label}` problem detail, `/comp/{slug}/leaderboard`. Wire SPA mount points (`/comp/*`) into router.go alongside the existing `/spa-test/*` mount.
- **1h — Staff UI.** SvelteKit: `/staff/comp` list, `/staff/comp/new`, `/staff/comp/{slug}` admin dashboard, problems editor, registrations, verify queue.
- **1i — Big-screen + magic-link UI + polish.** `/comp/{slug}/big-screen` SSE-driven leaderboard with rank-change animation; magic link request/verify pages; CSP for the SPA group; end-to-end smoke test (register → log attempts → see leaderboard update live).

### Phase 2 — Polish between league weeks

- Second and third scoring rules.
- CSV import for problem sets.
- Verify / override workflow in staff UI.
- CSV export of results.
- QR codes on each problem (printed by setters; link to `/comp/{slug}/p/{label}`).
- Comp visibility settings (public/members/registered).

### Phase 3 — Series machinery

- Aggregation across `competition_events` (sum, sum_drop_n, weighted finals).
- Season standings page.
- Climber comp history on profile (this triggers the next module to migrate to SvelteKit, probably).

---

## Conventions

Backend conventions unchanged from CLAUDE.md.

New conventions for the SPA:

- TypeScript strict mode, no `any`.
- Components use Svelte 5 runes (`$state`, `$derived`, `$effect`).
- API access only through generated client in `lib/api/client.ts`. No direct `fetch()`.
- Stores are typed and exported from `lib/stores/`.
- Component file naming: PascalCase (`ScoreChip.svelte`).
- Route file naming: SvelteKit conventions (`+page.svelte`, `+layout.svelte`).
- Test components with Vitest + Testing Library.
- Run `npm run check` (svelte-check) in CI.

---

## Open questions

1. ~~**Table names**~~ — RESOLVED: `locations`, `routes`, `users` confirmed.
2. **Scoring rule for the league** — which of `top_zone`, `fixed`, `decay` for week 1? Owner: Chris. Decision goes in `competitions.scoring_rule` for the league comp.
3. **Series shape** — one comp with N events for the whole league (recommended), or one comp per night? Schema supports both via `format` + `aggregation`. Owner: Chris.
4. ~~**Magic link infra**~~ — RESOLVED: SMTP exists for other purposes; no transactional auth flow today. Building it as a real Phase 1 work item, additive to password login.
5. ~~**Leaderboard visibility default**~~ — RESOLVED: `public` default. Staff can lock down per-comp via the column.
6. ~~**Existing worker**~~ — RESOLVED: no `cmd/worker/` binary. Background work runs in-process via `internal/jobs/queue.go`. Comp module touches none of it.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| SPA introduces complexity solo dev can't sustain | Phase 0 establishes the pattern in isolation; if it feels wrong, abort before Phase 1 starts. |
| Self-score abuse | Audit log on every action; staff verify endpoint; optional witness mode; rate limit per registration. |
| Magic link infra not in place | Phase 1 may need a small detour to add transactional email. Flag early. |
| SSE behind a proxy that buffers | Fly.io is fine; if you ever put nginx in front, set buffering off. |
| Scoring rule disagreement mid-season | Rule + config on the comp; recomputation is on read so changing rule re-ranks instantly. Document for staff. |
| Bib number reuse after withdrawal | Partial unique index handles this. |
| Idempotency key collision | UUIDv4 client-generated; collision probability negligible. Server treats key match as "already applied" — safe even on accidental reuse. |

---

## First commit suggestion

Phase 0 in one PR: SvelteKit scaffold, Go embed, build targets, smoke test at `/spa-test`. Zero comp logic. Lands the architecture decision in code so Phase 1 can proceed without revisiting it.

---

## What's deliberately deferred

Naming what's not in the plan, so we don't accidentally pretend it is:

- Service Worker / true offline support
- Walk-in registration
- Native mobile app
- Multi-region / horizontal scaling (in-process SSE hub assumes single instance)
- Materialized score caching (not needed at comp scale; revisit if reads get slow)
- WebSocket bidirectional channels (SSE is enough)
- Migrating other Routewerk modules to SvelteKit (will happen organically)
