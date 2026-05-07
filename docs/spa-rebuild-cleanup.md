# SPA rebuild — cleanup audit

Phase 2 of the rebuild is complete. The SvelteKit shell at `web/spa/`
now owns the entire user-facing surface and is mounted at the root URL
(`/walls`, `/routes`, `/sessions`, …) since PR #98. The HTMX templates
and handler methods that mirror SPA pages are still present but
**orphaned** — no router registration reaches them.

This doc tracks each HTMX surface as one of:

- **mirrored** — SPA equivalent exists; HTMX template + handler can be
  deleted in the cleanup PR. The router registration was already
  removed in #98.
- **kept** — HTMX surface still in use, either by design (auth flows,
  app-admin) or because the SPA links to it as a server-rendered
  artifact (route cards, card-batch PDFs).

> SPA shell + parity PRs:
> #50–#53 (Phase 1h staff comp), #54 head_setter authz,
> #55 shell + palette, #56 walls, #57 routes browser, #58 routes detail,
> #59 sessions list, #60 card batches, #61 profile + settings, #62 team,
> #63 quests browse, #64 audit doc, #65 role resolution, #66 palette
> revert + sidebar, #67 self-demotion guard, #68 climber log+rate,
> #69 notifications, #70 dashboard stats, #71 wall archive,
> #72 session lifecycle, #73 gym settings, #74 org admin,
> #75 progressions admin, #77 comp 404 + sessions hang + team org_admin
> + settings links, #78 comp shell + gym-aware route form, #79 hide
> quests for climbers + dashboard wall grid, #80 view-as system, #81
> photo upload, #82 view-as page gates + head_setter gym settings,
> #83 community tags + difficulty consensus, #84 archive view + badge
> showcase, #85 sidebar no-scroll, #86 per-ascent edit, #87 session
> publish, #88 org-wide team toggle, #89 progressions toggle, #90 staff
> stay on quests/badges when off, #91 card batch edit, #92 palette
> presets, #93 activity feed, #94 audit refresh, #95 quest create/edit
> form, #96 session photos + per-route ops, #97 setter playbook editor,
> #98 SPA promoted to root, /app/* → /* 308 redirects.

---

## Status by HTMX surface

### Auth (kept — by design)

| HTMX route | Status |
|---|---|
| `GET /login` + `POST /login` | kept — password auth; SPA `/sign-in` is magic-link only |
| `GET /register` + `POST /register` | kept — new account creation |
| `GET /verify-magic` | kept — magic-link callback (server-rendered cookie set) |
| `GET /setup` + `POST /setup` | kept — first-run org bootstrap |
| `GET /join-gym` etc. | kept — climber onboarding |
| `POST /logout` | kept — cookie clear + redirect, no JS |

### Server-rendered artifacts (kept — SPA links to these)

| HTMX route | Status |
|---|---|
| `GET /routes/{id}/card/print.{png,pdf}` | kept — SPA route detail embeds + downloads |
| `GET /routes/{id}/card/share.{png,pdf}` | kept — same |
| `GET /card-batches/{id}/download.pdf` | kept — SPA card-batch detail download |
| `GET /card-batches/{id}/cutlines.dxf` | kept — laser-cutter file download |
| `GET /card-batches/{id}/preview.png` | kept — SPA card-batch preview <img> |

### App admin (kept — server-rendered observability)

| HTMX route | Status |
|---|---|
| `GET /admin/health` | kept — pool / DB / queue dashboard |
| `GET /admin/metrics` | kept — request counters dashboard |

### Climber-facing (mirrored — handlers + templates can be removed)

| HTMX route | SPA equivalent |
|---|---|
| `GET /` (redirect) | `/` (SPA dashboard) |
| `GET /dashboard` | `/` (#70, #79) |
| `GET /routes` | `/routes` |
| `GET /archive` | `/archive` (#84) |
| `GET /routes/{id}` | `/routes/[id]` — log ascent (#68), rate, photo upload (#81), community tags (#83), difficulty consensus (#83) |
| `POST /routes/{id}/ascent` | `POST /api/v1/locations/{loc}/routes/{id}/ascent` |
| `POST /routes/{id}/rate` | `POST /api/v1/locations/{loc}/routes/{id}/rate` |
| `POST /routes/{id}/difficulty` | `POST /api/v1/locations/{loc}/routes/{id}/difficulty` (#83) |
| `POST /routes/{id}/tags`, `/tags/remove`, `/tags/delete` | `/api/v1/locations/{loc}/routes/{id}/tags` (#83) |
| `POST /routes/{id}/photos`, `/photos/{id}/delete` | `/api/v1/locations/{loc}/routes/{id}/photos` (#81) |
| `GET /routes/{id}/ascents-feed` | orphan — was an HTMX-only live ticker; SPA route detail polls `listRouteAscents` instead |
| `GET /profile`, `/profile/settings`, `POST /profile/*` | `/profile`, `/settings` |
| `GET /profile/ticks/{id}/edit`, `POST .../{id}`, `/delete` | inline edit + delete on `/profile` (#86) |
| `GET /notifications`, `POST .../*` | `/notifications` (#69) |
| `GET /quests`, `/quests/mine`, `/quests/{id}` | `/quests/*` |
| `POST /quests/{id}/{start,log,abandon}` | JSON helpers |
| `GET /quests/badges` | `/quests/badges` (#84) |
| `GET /quests/activity` | `/quests/activity` (#93) |

### Setter / staff-facing (mirrored)

| HTMX route | SPA equivalent |
|---|---|
| `GET /routes/manage`, `/routes/new`, etc. | `/routes/*` — gym-aware pickers + auto strip-date (#78) |
| `GET /walls/*`, archive/unarchive/delete | `/walls/*` (#71) |
| `GET /sessions/*`, full lifecycle | `/sessions/*` (#72) |
| `POST /sessions/{id}/publish` | one-shot publish on `/sessions/[id]` (#87) |
| `GET /sessions/{id}/photos` + `POST /sessions/{id}/routes/...` | `/sessions/[id]/photos` (#96) |
| `GET /card-batches/*` (list, new, detail, edit, retry, delete) | `/card-batches/*` |
| `POST /card-batches/{id}/edit` | inline edit on `/card-batches/[id]` (#91) |
| `GET /settings`, gym settings + circuits + hold-colors | `/settings/gym` (#73), head_setter+ writes (#82) |
| `POST /settings/palette-preset` | one-click presets in `/settings/gym` (#92) |
| `POST /settings/progressions-toggle` | `/settings/gym` toggle card (#89) |
| `GET /settings/team` | `/team` |
| `POST /switch-view-as` | sidebar view-as bar (#80) + page gates (#82) |
| `POST /switch-location` | sidebar location picker — uses local store, no server cookie |
| `GET /settings/organization`, gym CRUD | `/settings/org` (#74) |
| `GET /settings/organization/team` | "Whole org" toggle on `/team` (#88) |
| `GET /settings/progressions`, domain/badge CRUD | `/settings/progressions` (#75); quest create/edit form (#95) |
| `GET /settings/playbook` + edits | `/settings/playbook` (#97) |

### Quest manual mark-complete (kept — narrow recovery path)

| HTMX route | Status |
|---|---|
| `POST /quests/{id}/complete` | kept — staff-only manual override; auto-complete fires from the quest service for normal flows |

---

## Cleanup work remaining

After this PR (#98), the swap-to-root and route deletes are done. The
remaining cleanup is template + dead-handler removal:

1. **Delete orphaned HTMX handlers** in `internal/handler/web/` for the
   surfaces marked **mirrored**. The router no longer points at them so
   they're safe to remove. Skim for any cross-call from a kept handler
   (`SetupSubmit` → `loginSubmit`-style helpers) before deleting.
2. **Delete orphaned templates** in `web/templates/` for the same set.
   Watch for partials reused by kept pages — e.g. `partials/route-card.html`
   feeds the magic-link callback's success page and the route-card PDF
   pipeline. Don't delete partials without `grep -r` first.
3. **Delete the `_app/*` dead-code from webHandler/NewHandler** — once
   handlers are gone, the constructor parameters that fed them can come
   off too. Many repos / services will become unused on the HTMX side
   but stay on the API side; only remove from `webhandler.NewHandler`,
   not from `repository.New*` calls in `router.go`.

Two follow-up PRs (handler/templates separately, then constructor
slimdown) keep each diff reviewable. Reversible by reverting the commit
if the user spots a regression.
