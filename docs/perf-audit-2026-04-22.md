# Routewerk Performance Audit — 2026-04-22

Scope: Go API server, HTMX web frontend, and embedded static assets.
Primary focus: performance. Notable security and best-practice findings are
captured at the end.

Connection pool context (`internal/database/db.go:41`): **MaxConns = 5** per
app instance on Fly.io shared Postgres. Every finding below should be read
against that budget — queries that look cheap in isolation still contend with
the same five connections across all concurrent requests.

## Severity key

- **Critical** — user-visible latency or a hard request-per-second ceiling on
  the current pool budget. Fix first.
- **High** — notable wasted work on a hot path; worth prioritising before the
  next traffic bump.
- **Medium** — contributes real overhead but not a ceiling today.
- **Low** — polish / future-proofing.

## Memory-impact convention

The app runs on a **256 MB Fly VM** (see `fly.toml`). RSS headroom is real
budget. Each finding below carries a **Memory:** line:

- **Neutral** — no meaningful RSS change.
- **Reduces RSS** — fix lowers memory usage, usually by removing a buffer
  or deferring work off the request path.
- **+N KB / +N MB steady-state** — rough estimate of additional baseline
  RSS the fix introduces, with a sizing handle.

If you scale up memory, treat `+` values as "comfortably fits"; if you stay
on the current tier, size the handles explicitly.

---

## 1. Authenticated page load fires 6+ serial DB round-trips before the handler starts — Critical

`internal/middleware/websession.go:124-161` and
`internal/handler/web/helpers.go:83-117`.

Every request to any authenticated web page does the following, serially, on
the request goroutine (each is its own pool acquire):

| # | Call site | Query |
|---|-----------|-------|
| 1 | `websession.go:124` | `WebSessionRepo.GetByTokenHash` |
| 2 | `websession.go:149` | `UserRepo.GetByID` |
| 3 | `websession.go:158` | `UserRepo.GetMemberships` |
| 4 | `helpers.go:93`     | `LocationRepo.GetByID` (active location) |
| 5 | `helpers.go:101`    | `LocationRepo.ListForUser` (switcher) |
| 6 | `helpers.go:111`    | `NotificationRepo.UnreadCount` |

Then the handler runs (dashboard adds 4 more queries; the routes list adds
2 — count + select). So the **/dashboard** hot path is ~10 round-trips per
view, serialised on a pool of 5 connections. With even moderate concurrency,
requests queue on pool acquire before any handler code runs.

**Fix options, cheapest first:**

1. Collapse items 1–3 into one query:
   ```sql
   SELECT s.*, u.*, m.role, m.org_id, m.location_id
   FROM web_sessions s
   JOIN users u ON u.id = s.user_id
   LEFT JOIN user_memberships m
     ON m.user_id = u.id AND m.deleted_at IS NULL
   WHERE s.token_hash = $1 AND s.expires_at > NOW();
   ```
   That alone cuts the hot path by two round-trips.
2. Collapse items 4–6 into a single query that joins `locations` to
   `notifications` (COUNT filter) and filters memberships by user. Even a
   one-CTE query with three result columns is cheaper than three separate
   statements.
3. Run a short in-process cache (`sync.Map` + 30s TTL, keyed by
   `session_id`) for (user, memberships, user-locations). The session table is
   already a cache of bearer credentials; nothing here is privilege-sensitive
   beyond revocation latency. Unread count can stay live.
4. `notifRepo.UnreadCount` is unnecessary for the first paint — move it
   behind an HTMX poll on the nav badge so it doesn't block page HTML.

**Memory:** Fix 1 (query collapse) and Fix 2 are **neutral** — same rows, one
connection instead of three. Fix 4 is **neutral** (moves work to a later
request, same total). Fix 3 (in-process cache) is **+≈10 MB steady-state at
10k active sessions** (≈1 KB entry × cap). Bound it with an LRU and a ceiling
(`lru.New(5_000)` → ~5 MB cap). Skip this option entirely if you're staying
on the 256 MB tier — Fixes 1, 2, and 4 already cover most of the win.

---

## 2. `List` + `ListWithDetails` run a separate `COUNT(*)` query per request — High

`internal/repository/route.go:230-234, 297-301`,
`internal/repository/ascent.go:123`, `internal/repository/user.go:345, 419`.

Every paginated listing does a full `COUNT(*)` over the filtered set, then
immediately re-runs essentially the same scan with `LIMIT/OFFSET`. Two round-
trips, two plan executions, two trips through the same index/table.

Replace with `COUNT(*) OVER ()` in the same SELECT:

```sql
SELECT r.id, ..., r.updated_at, COUNT(*) OVER () AS total
FROM routes r
WHERE ...
ORDER BY r.date_set DESC, r.created_at DESC
LIMIT $N OFFSET $N+1
```

For large filtered sets PG will still scan the same rows, but one query plan
instead of two. Halves the round-trips on every listing page (routes,
archive, member search, ascent history).

Also: `LIMIT/OFFSET` pagination on `routes` over `ORDER BY date_set DESC,
created_at DESC` starts to cost real work past a few thousand rows. The
partial index `idx_routes_location_status_type ... WHERE deleted_at IS NULL`
helps the filter, but not the sort. Add a keyset-pagination variant (`WHERE
(date_set, created_at, id) < ($1, $2, $3)`) for the infinite-scroll endpoints
in `climber/routes.html` — it avoids OFFSET scan entirely and stays constant
cost.

**Memory:** **Neutral.** Adds one `int8` column per result row (8 bytes ×
page size, ≤50 rows by default). Keyset pagination is also memory-neutral —
no server-side cursor, just a `WHERE` predicate.

---

## 3. Per-row `tx.Exec` inside route-tag write paths — High

`internal/repository/route.go:127-131, 496-500`,
`internal/repository/route_skill_tag.go:36-43` (per Explore agent).

`CreateWithTags` and `SetTags` do one `INSERT INTO route_tags (route_id,
tag_id) VALUES ($1,$2)` per tag inside a `for` loop. Under a transaction
each round-trip still acquires a lock and waits on network; 5 tags = 5
round-trips.

Use a single multi-row insert with `unnest`:

```sql
INSERT INTO route_tags (route_id, tag_id)
SELECT $1, UNNEST($2::uuid[])
ON CONFLICT DO NOTHING;
```

Pass `tagIDs` as a `[]string` — pgx binds it as a Postgres text/uuid array
natively. Same fix applies to `route_skill_tag.SetTags`. On route creation
with 4 tags this drops 4 queries to 1 on every save.

**Memory:** **Neutral.** One array-bound arg on the wire; fewer
pgx `Exec` allocations than the current per-row loop. Slightly better under
concurrent saves.

---

## 4. `cardbatch.Service` hydrates walls and setters one-at-a-time — Considered, rejected

`internal/service/cardbatch/cardbatch.go:114-152`.

Initial read: replace the per-route `walls.GetByID` / `users.GetByID` loop
with two batch lookups up front.

Rejected on second pass. The 256 MB Fly VM is the binding constraint (see
the `MaxBatchCards = 200` comment at `cardbatch.go:20-34`): gofpdf buffers
the whole PDF in memory before `Output()`, so peak RSS during a render is
already close to the headroom on that tier. The current stream-through-
the-loop pattern with the per-call `wallNames` / `setterNames` maps keeps
the working set at "one route's worth of metadata at a time"; eagerly
materialising all routes + walls + setters before starting the composer
would raise the ceiling on concurrent batch renders.

The serial fetches also ride along the same per-route `GetByID` that's
already in the loop for `routes`, so the extra round-trips are small
compared to the PDF render cost. Leaving as-is.

---

## 5. `RateLimiter` cleanup is deferred, map can exceed `maxClients` — Medium

`internal/middleware/security.go:235-261`.

The cap (`defaultMaxClients = 10_000`) is enforced in `LimitByKey` on insert,
but cleanup only runs on a ticker every `window` (1 min for web, 1 hour for
batch-create). Under a burst of unique keys (login scanning, credential
stuffing, a flaky CDN path varying X-Forwarded-For) the map can stay near
cap for the whole ticker interval, and each `evictOldest` scan is O(N).

Fix in order of cost:

1. Run `cleanup()` opportunistically on every N-th insert (e.g. every 100).
2. Replace `evictOldest` with a min-heap indexed by `windowStart`, so
   eviction is O(log N) instead of O(N).
3. Consider `golang.org/x/time/rate.Limiter` per key with an LRU — it's
   allocation-light and gives a clean token-bucket.

The batch-creation limiter keyed by user ID is lower risk (there aren't
millions of users), but the login and generic web limiters are keyed by IP
and see the widest key diversity.

**Memory:** Fix 1 (opportunistic cleanup) and Fix 2 (min-heap) are
**neutral** — same entry count at `maxClients=10_000`, different index
structure. Fix 3 (`x/time/rate`) is **+≈160 KB steady-state** at cap
(`rate.Limiter` is ~48 B vs current ~32 B × 10 000). Prefer Fix 1 or 2 on
the 256 MB tier; Fix 3 is fine once you've scaled up.

---

## 6. `http.FileServer` on embedded FS emits no `Cache-Control`, no `ETag`, no `Last-Modified` — Medium

`internal/handler/web/static.go:62-69`,
`internal/middleware/security.go:119-125`.

`SecureHeadersStatic` *does* set `Cache-Control: public, max-age=31536000,
immutable` — that part is correct. The issue is repeat visitors and shared
caches:

- `embed.FS` returns a zero modtime, so `http.FileServer` emits no
  `Last-Modified`. Combined with no `ETag`, browsers that revalidate (any
  cache flush, shared proxy, private mode) do a full re-download rather than
  a 304.
- `Cache-Control: immutable` relies on the `?v=hash` suffix to bust caches,
  which is correct — but older clients and some proxies do a `max-age=0`
  revalidation anyway.

Fix: wrap the handler, compute a content hash at startup (same pass you
already do in `initAssetHashes`), and set `ETag: "<hash>"` on the response.
Handle `If-None-Match` and return 304 if matched. Cheap one-time setup, turns
revalidations into 304s on the network.

Bonus: `app.js` and `routewerk.css` (~111 KB unminified) would benefit from a
minification step in the Dockerfile — `esbuild` is a single binary and takes
a few hundred ms. Keeps the binary self-contained and halves static payload.

**Memory:** **Neutral.** The hashes are already computed and held by
`initAssetHashes` (~8 bytes each × ~20 assets = ~160 bytes). Adds a small
ETag header string per response. Minification happens at build time — zero
runtime cost, and the smaller embedded assets actually **reduce** binary
RSS by a few hundred KB.

---

## 7. Gzip middleware compresses binary assets and has no min-size threshold — Medium

`internal/middleware/security.go:170-192`, `internal/router/router.go:178`.

`middleware.Gzip` is applied to the `/static/*` group, which serves PNG,
JPEG, WOFF2, and the already-minified `htmx.min.js`. There is no content-type
or size gate:

- Gzipping PNG/JPEG/WOFF2 wastes CPU for zero payload win — they're already
  compressed.
- Gzipping a 200-byte 404 response is also net-negative.
- The middleware also doesn't check `Content-Encoding` on the response
  before wrapping, so a handler that pre-gzips would get double-compressed.

Fix:

```go
ct := w.Header().Get("Content-Type")
if !gzippableType(ct) || w.Header().Get("Content-Encoding") != "" {
    next.ServeHTTP(w, r)
    return
}
```

Allow-list: `text/html`, `text/css`, `application/javascript`,
`application/json`, `image/svg+xml`. Add a ~1 KB minimum body size if you
want; HTMX partial swaps are usually larger than that already.

**Memory:** **Reduces RSS.** Skips allocating a `gzipResponseWriter` wrapper
and a pooled `gzip.Writer` for every PNG/JPEG/WOFF2 request. Net win under
load.

---

## 8. Template `shared` layout is re-built as string concat at startup — Low

`internal/handler/web/render.go:22-88`.

`loadTemplates` concatenates `base.html`, `partials/sidebar.html`, and
`partials/route-card.html` into a single string, then for every one of ~40
pages calls `template.New(page).Parse(shared)` followed by a second
`Parse(pageBytes)`. The shared layout is parsed 40× from its source bytes.

This is a startup-only cost (~tens of ms), not per-request, so impact is low.
But it's easy to switch to `template.ParseFS`-style composition or compile
`shared` once into a `*template.Template` and `Clone()` it per page. Clone is
cheap and keeps each page's tree independent.

**Memory:** **Neutral.** Same number of compiled template trees held in the
`h.templates` map at the end of `loadTemplates`; only the build path
changes.

---

## 9. HTMX and `app.js` block paint in `<head>` — Medium

`web/templates/base.html:8-9` (per Explore agent).

Both `<script src="/static/js/htmx.min.js">` and app.js are parser-blocking
`<script>` tags with no `defer`. On cold cache, paint waits for both to
download and execute. For HTMX-heavy pages this is real — ~50 KB + ~40 KB on
the critical path.

Add `defer` to both, or move them to the end of `<body>`. HTMX attributes on
elements are read after DOMContentLoaded, so `defer` is correct and safe.

**Memory:** **Neutral.** Template-only edit; nothing changes server-side.

---

## 10. `initAssetHashes` walks embedded FS on first `StaticPath` call — Low

`internal/handler/web/static.go:22-45`.

Guarded by `sync.Once`, so it runs exactly once. First template render of a
cold process pays the walk cost (dozens of ms on a shared-CPU VM). Move the
`initAssetHashes()` call to package `init()` or into `loadTemplates` so the
first real request doesn't pay the one-time cost.

**Memory:** **Neutral.** Same hash map allocated; only the timing of the
walk changes.

---

## 11. `Logger` / `Recovery` / `Metrics` middleware body wrapping — Low, verify

`internal/router/router.go:43-44`. I didn't read `middleware/logging.go` or
`metrics.go` end-to-end — worth a look. Specifically: does the response
writer wrapper buffer the whole body? If so, HTMX partials (which can be
large fragments) are fully held in memory before being written. The metrics
middleware should only need `WriteHeader` to record the status code, and
`Write` to increment a byte counter — no buffering required.

**Memory:** **Potentially reduces RSS** if a wrapper is buffering. Worth
verifying; if so, the fix is a net win.

---

## 12. Background cleanup goroutines use long tickers with no jitter — Low

`cmd/api/main.go:157, 185`.

`cleanupExpiredSessions` (1 hour) and `cleanupOldCardBatches` (24 hours)
fire at exactly the same offset on every replica. With two app instances,
both do the DELETE at the same wall-clock moment on the same Postgres.
Low-impact today (both DELETEs are cheap and indexed), but easy to jitter:

```go
time.Sleep(time.Duration(rand.Int63n(int64(5*time.Minute))))
ticker := time.NewTicker(1 * time.Hour)
```

**Memory:** **Neutral.**

---

## 13. Event bus can spawn unbounded goroutines for async handlers — Medium, verify

Per Explore agent, `internal/event/memory.go` dispatches each async handler
on a fresh goroutine with no worker pool. I didn't read this file myself —
worth confirming. If a single request enqueues N events and there are M
async subscribers, that's N×M goroutines. Under a burst (e.g. a bulk import
enqueuing 1000 activity events), this is a real goroutine spike.

Fix: a bounded worker pool with a buffered channel. `golang.org/x/sync/errgroup`
with `SetLimit` is the minimal-dependency version.

**Memory:** **Reduces RSS under bursts** (caps peak goroutine count) but
adds a small steady-state floor. Sizing handle: `bufferedChannelSize *
avgEventSize + workerCount * ~8KB stack`. Example: 1 000-slot channel ×
~100 B event + 10 workers × 8 KB = **≈180 KB steady-state**. Keep the
channel bounded (don't size it in the millions) and this stays small on
the 256 MB tier.

---

## Notable security / best-practice findings (captured while auditing perf)

Not exhaustive — this pass focused on performance — but worth filing.

### S1. CSP allows `'unsafe-inline'` for styles — Low

`internal/middleware/security.go:66`. Every other directive is tight
(`default-src 'none'`, `script-src 'self'`, `form-action 'self'`), but
`style-src 'self' 'unsafe-inline'` defeats the purpose for style-based
injection (data exfil via background-image URLs, layout-shift tracking,
etc.). Move the handful of inline `style="..."` attributes to classes or to a
hashed inline block with `'sha256-...'`, then drop `'unsafe-inline'`.

**Memory:** **Neutral.** Header-string change only.

### S2. `safeCSS` trusts the caller — Medium

`internal/handler/web/web.go:177-179`. The template func returns
`template.CSS(s)` without validating the string. It's currently only called
on values that flow through `sanitizeColor` upstream, but nothing in the
type system enforces that — a future caller that hands in user input
unchecked would render as literal CSS. Tighten the function:

```go
func safeCSS(s string) template.CSS {
    if !cssColorRe.MatchString(s) { return "" }
    return template.CSS(s)
}
```

**Memory:** **Neutral.** One compiled `*regexp.Regexp` at package init
(<1 KB).

### S3. Non-multipart handlers don't bound `ParseForm` input — Medium

Go's `http.Request.ParseForm` defaults to `defaultMaxMemory = 32 MB` for
memory and is bounded by `Server.MaxBytesReader` only if the caller sets it.
We do `srv.MaxHeaderBytes = 1 << 20` (good) but not a body cap. Add
`r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` at the top of form-POST
handlers, or install a single middleware that does it for the web group.
Prevents a misbehaving or malicious client from sending a 50 MB form body.

**Memory:** **Reduces RSS.** Caps per-request body parsing at 1 MB vs Go's
32 MB in-memory default for `ParseForm`. Direct hit to worst-case RAM under
abuse.

### S4. `RequireSession` returns 401 to HTMX but 303 to full pages on missing session, but renders 500 on `sessions.GetByTokenHash` error — Low

`internal/middleware/websession.go:126-134`. Logging the error and redirecting
to login is correct UX, but on a pool-exhaustion path (query timeout, etc.)
the user silently bounces to `/login` instead of seeing a 5xx. Acceptable
default, but consider returning a 503 when the error is `context.DeadlineExceeded`
so load balancers can back off instead of hammering.

**Memory:** **Neutral.**

### S5. Refresh endpoint accepts expired access tokens by design — None, noted

`internal/router/router.go:402-406` plus `middleware/auth.go:54-80`.
`AuthenticateAllowExpired` explicitly allows expired JWTs through to
`/api/v1/auth/refresh`. This is intentional and correct for refresh flow —
signature is still verified. Just flagging so reviewers don't read it as a
bug. Keep the authLimiter in front (already there) so refresh abuse is
bounded.

**Memory:** N/A (no change proposed).

### S6. `AccessLog` / `AuditService` writes on the request path — Low, check

Most `h.audit.Log(...)` calls I spot-checked are awaited inline (route
create, tag delete, org update). If the audit write is slow or the audit
table is hot, every mutating request waits on it. Consider moving to
`jobQueue.Enqueue` for audit — you already have the queue plumbed for
emails and notifications, and audit is the textbook use case.

**Memory:** **Reduces RSS on the request path.** Audit payload is held in
the queue row instead of a long-lived request goroutine stack. The queue is
already in place; this doesn't add infrastructure.

### S7. `bcrypt.DefaultCost` is 10, not 12 — None, noted

Per the services Explore agent: the Go stdlib's `bcrypt.DefaultCost` is 10.
That's fine for today — NIST guidance still considers 10 acceptable for
bcrypt and it keeps login under ~50ms. No change needed unless you bump CPU.

**Memory:** N/A.

---

## Quick-win priorities

If you only had a day:

1. **#1 — serialised auth queries.** Collapse the three websession queries
   into one and defer `UnreadCount` to an HTMX poll. Biggest single win on
   p50 page latency and pool contention.
2. **#2 — `COUNT(*) OVER ()`.** Mechanical change across ~5 repo methods.
3. **#3 — `unnest` for route tags.** One-line rewrite of two functions.
4. **#7 — Gzip content-type gate.** 10 lines in `security.go`.
5. **#9 — `defer` on the script tags.** One-line template edit.

(Skipping #4 — retained on purpose to keep RSS flat under the 256 MB Fly VM
ceiling; see that section.)

Together those are low-risk, easy to review, and meaningfully change the
shape of the request path.

## Things the code gets right (worth protecting)

- Templates compiled at startup, not per-request (`render.go`).
- `ListActiveByLocation` is explicitly called out as avoiding N+1
  (`route.go:355`).
- `pgxpool` sized for Fly.io shared Postgres with conservative defaults
  (`db.go:39-47`).
- `sync.Pool` on gzip writers and CSRF tokens — correct usage.
- Server timeouts set (read/write/idle) at
  `cmd/api/main.go:123-128` — protects against slowloris.
- Prepared parameters everywhere (no string-interpolated SQL spotted).
- CSP is `default-src 'none'` with explicit allow-lists. Good default.
- Double-submit CSRF with `SameSite=Strict` and per-boundary rotation.
- Slow query tracer wired up (`database/querylog.go`).
- `RequestTimeout` middleware caps per-request work globally.

None of these need to change; they just shouldn't regress.
