# SPA rebuild — cleanup audit

After Phases 2.1 → 2.8, the new SvelteKit shell at `/app/*` covers most
of the existing HTMX surface. This doc audits each HTMX route and
classifies it: **mirrored** (a SPA equivalent exists at `/app/*`),
**unique** (still the only home for that feature, leave as-is for now),
or **server-rendered** (must stay as HTMX/server output, not a UI
candidate).

The recommendation at the bottom is what to delete now vs. defer. Nothing
here is destructive — actually deleting templates + handlers happens in a
follow-up PR after you sign off.

> Phase order: 2.1 shell + palette · 2.2 walls · 2.3 routes ·
> 2.4 sessions · 2.5 card batches · 2.6 profile + settings ·
> 2.7 team · 2.8 quests.

---

## Status by HTMX surface

### Climber-facing

| HTMX route | Status | SPA equivalent | Notes |
|---|---|---|---|
| `GET /` | redirect | — | redirects logged-in users to `/dashboard`; once `/app/*` swaps to root this becomes the SPA shell |
| `GET /login` + `POST /login` | unique | partial | HTMX password auth; SPA `/sign-in` is magic-link only. Both still useful — different auth flows. |
| `GET /register` + `POST /register` | unique | — | new account creation, HTMX-only |
| `GET /verify-magic` | server-rendered | — | callback for magic-link emails; sets cookie + redirects. Cannot become SPA. |
| `GET /setup` + `POST /setup` | unique | — | first-run org bootstrap; never SPA-worthy |
| `GET /join-gym` etc. | unique | — | climber-onboarding flow, not migrated |
| `POST /switch-location` | unique | — | HTMX-side helper; SPA has its own location store (`location.svelte.ts`) |
| `POST /switch-view-as` | unique | — | dev/admin "view as" affordance |
| `GET /routes` | unique | — | climber route browser; SPA `/app/routes` is the *staff* browser. Climber view (filtered to active, with tag chips, ascent CTA) hasn't been migrated. |
| `GET /archive` | unique | — | climber archive view |
| `GET /routes/{id}` | unique | partial | SPA `/app/routes/{id}` is staff-flavored (status toggle, edit, delete). Climber detail has log-ascent + rate + community tags + photos. Not migrated. |
| `GET /routes/{id}/card/print.{png,pdf}` | server-rendered | — | rendered server-side via `cardGen`. Not a UI surface. |
| `GET /routes/{id}/card/share.{png,pdf}` | server-rendered | — | same |
| `POST /routes/{id}/ascent` | unique | — | climber log-ascent endpoint, called from the climber route detail. Not migrated. |
| `GET /routes/{id}/ascents-feed` | unique | — | live ascent feed partial used by the route detail page |
| `POST /routes/{id}/rate`, `/difficulty`, `/tags`, `/photos` | unique | — | climber interactions on the route detail; all HTMX-only |
| `GET /profile`, `/profile/ticks` | mirrored | `/app/profile` | SPA covers it ✓ |
| `GET /profile/settings`, `POST /profile/settings`, `POST /profile/password` | mirrored | `/app/settings` | covered ✓ |
| `GET /profile/ticks/{ascentID}/edit`, `POST .../{ascentID}`, `POST .../delete` | unique | — | per-tick edit/delete from the profile, not migrated |
| `POST /logout` | server-rendered | — | clears cookie + redirects. Sidebar links to it directly. |
| `GET /notifications`, `/notifications/badge` | unique | — | climber notifications inbox, not migrated |
| `POST /notifications/...` | unique | — | mark-read endpoints, not migrated |
| `GET /quests`, `/quests/mine`, `/quests/{id}` | mirrored | `/app/quests`, `/app/quests/{id}` | covered ✓ |
| `POST /quests/{id}/start`, `/log`, `/abandon` | mirrored | `POST /api/v1/quests/...` | new JSON endpoints in 2.8 |
| `POST /quests/{id}/complete` | unique | — | manual mark-complete (auto-complete fires via service when target hit). SPA has no UI for this. |
| `GET /quests/badges` | unique | — | badge showcase grid, not migrated |
| `GET /quests/activity` | unique | — | quest activity feed, not migrated |

### Staff-facing

| HTMX route | Status | SPA equivalent | Notes |
|---|---|---|---|
| `GET /dashboard` | partial | `/app` | SPA landing is quick-action cards. The HTMX dashboard has stats, routes-by-wall grid, and an activity feed — none of which are in the SPA yet. |
| `GET /routes/manage` | partial | `/app/routes` | SPA browser has filters; HTMX has the same plus bulk-archive |
| `GET /routes/new`, `/routes/new/fields`, `POST /routes/new` | mirrored | `/app/routes/new` | ✓ |
| `GET /routes/{id}/edit`, `POST .../edit`, `POST .../status` | mirrored | `/app/routes/{id}` (inline edit + status toggle) | ✓ |
| `GET /walls`, `/walls/new`, `POST /walls/new` | mirrored | `/app/walls`, `/app/walls/new` | ✓ |
| `GET /walls/{id}`, `/walls/{id}/edit`, `POST /walls/{id}/edit` | mirrored | `/app/walls/{id}` | ✓ |
| `POST /walls/{id}/archive`, `/walls/{id}/unarchive` | unique | — | no JSON endpoint exists; SPA uses hard-delete instead. Backend work needed to add `PATCH /walls/{id}` accepting `{archived: bool}` if you want SPA parity. |
| `POST /walls/{id}/delete` | mirrored | DELETE in SPA | ✓ |
| `GET /sessions`, `/sessions/new`, `/sessions/{id}`, `/sessions/{id}/edit` | mirrored | `/app/sessions/*` | ✓ |
| `POST /sessions/{id}/assign`, `/unassign/...` | unique | — | needs team-list JSON endpoint to do setter selection. Stub on SPA links back to HTMX. |
| `POST /sessions/{id}/strip`, `/strip/.../delete` | unique | — | strip-targets are web-only |
| `POST /sessions/{id}/checklist/.../toggle` | unique | — | checklist is web-only |
| `POST /sessions/{id}/delete`, `/publish`, `/reopen`, `GET /complete` | unique | — | session lifecycle web-only |
| `GET /sessions/{id}/photos`, `POST /sessions/{id}/routes/...` | unique | — | session photos + per-session route operations |
| `GET /card-batches`, `/card-batches/new`, `/card-batches/{id}` | mirrored | `/app/card-batches/*` | ✓ |
| `GET /card-batches/{id}/edit`, `POST .../edit` | unique | — | SPA has no edit form (theme + cutter are pickable on create, batch is otherwise immutable on the JSON side) |
| `GET /card-batches/{id}/download.pdf` | server-rendered | linked from SPA | direct link from `/app/card-batches/{id}` ✓ |
| `GET /card-batches/{id}/cutlines.dxf`, `/preview.png` | server-rendered | — | downloadable artifacts, no UI |
| `POST /card-batches/{id}/delete` | mirrored | DELETE | ✓ |
| `GET /settings`, `POST /settings` | unique | — | gym settings: circuits + hold-colors + palette presets. Not migrated. |
| `POST /settings/circuits/...`, `/hold-colors/...`, `/palette-preset` | unique | — | child write endpoints |
| `GET /settings/team`, `POST /settings/team/{m}/role` | mirrored | `/app/team` | ✓ (also `DELETE /api/v1/memberships/{id}` for removal) |
| `GET /settings/playbook` + edit endpoints | unique | — | setter playbook editor |
| `POST /settings/progressions-toggle` | unique | — | feature flag toggle |
| `GET /settings/organization` + child writes | unique | — | org-level settings (logo, name, etc.) |
| `GET /settings/organization/gyms/...` | unique | — | location admin (create + edit gym) |
| `GET /settings/organization/team` | unique | — | org-wide team list (similar to `/app/team` but org-scoped) |
| `GET /settings/progressions` + child editors | unique | — | quest catalog admin (create/edit/deactivate quests, badges, domains) |
| `GET /admin/health`, `/admin/metrics` | unique | — | app-admin observability views |

### Competitions

The `/comp/*` and `/staff/comp/*` SPA surfaces (Phase 1g + 1h) have always
been SPA-only. No HTMX equivalents exist to delete.

---

## Recommendation: delete now (HTMX surfaces fully mirrored)

These are safe to delete in a follow-up PR — the SPA covers them with full
parity:

- `/profile`, `/profile/ticks`, `/profile/settings`, `POST /profile/settings`, `POST /profile/password`
- `/walls`, `/walls/new`, `POST /walls/new`, `/walls/{id}`, `/walls/{id}/edit`, `POST /walls/{id}/edit`, `POST /walls/{id}/delete`
- `/routes/new`, `/routes/new/fields`, `POST /routes/new`
- `/routes/{id}/edit`, `POST .../edit`, `POST .../status`
- `/sessions/new`, `POST /sessions/new`, `/sessions/{id}/edit`, `POST .../edit`
- `/card-batches`, `/card-batches/new`, `/card-batches/{id}`, `POST /card-batches/{id}/delete`
- `/settings/team`, `POST /settings/team/{m}/role`

Templates that go with them (under `web/templates/`):
`climber/profile.html`, `climber/profile-settings.html`,
`setter/walls.html`, `setter/wall-form.html`, `setter/wall-detail.html`,
`setter/route-form.html`, `setter/team.html`,
`setter/card-batches.html`, `setter/card-batch-form.html`,
`setter/card-batch-detail.html`, plus the partials they reference.

> ⚠ Be careful: some HTMX templates are co-rendered (the route browser
> includes route-card partials shared with the climber-facing route
> detail). Check imports before deleting.

## Recommendation: keep for now (still unique)

- All auth flows (`/login`, `/register`, `/verify-magic`, `/setup`, `/join-gym`)
- Climber-facing read views (`/routes`, `/archive`, `/routes/{id}` climber detail with ascent log + community tags + photos)
- Notifications inbox (`/notifications`, mark-read endpoints)
- Quests admin (`/quests/badges`, `/quests/activity`, `POST /quests/{id}/complete`)
- Setter dashboard rich content (`/dashboard` stats + routes-by-wall + activity feed)
- Wall archive/unarchive (no JSON yet)
- Session lifecycle, strip-targets, checklist, photos, per-session routes
- Card batch edit + cutlines + preview
- Gym settings (circuits, hold-colors, palette presets)
- Setter playbook + progressions toggle
- Organization settings + gym-create/edit
- Org-wide team management
- Progressions admin (quest/badge/domain CRUD)
- Admin observability (`/admin/*`)
- Server-rendered artifacts (route cards, card batch PDFs, magic-link callback, logout)

## Recommended next backend work to unblock more deletions

1. `PATCH /api/v1/locations/{id}/walls/{wallID}` accepting `{archived: bool}` → unblocks SPA archive parity.
2. `GET /api/v1/locations/{id}/team` already exists → wire into the SPA assignment form to enable session assignment management; then we can drop the HTMX `/sessions/{id}/assign` flow.
3. JSON endpoints for session lifecycle (`PATCH /sessions/{id}/status` accepting `planning|in_progress|complete|cancelled`).
4. JSON endpoints for the climber route view: ascent log (already have `POST /routes/{id}/ascent`), ratings (have `POST .../rate`), community tags. Mostly already there — just needs SPA UI work.

## Recommendation: when to swap `/app/*` to root

Hold off until:
1. The user has confirmed the new shell renders correctly with their data on `routewerk-dev`.
2. The "delete now" list above has actually been deleted (otherwise the swap is a routing trap — the HTMX templates would render 404s if a stale link points at them).
3. A redirect plan exists for the surfaces that still live at HTMX paths (e.g. `/profile` → `/app/profile`).

The router swap itself is mechanical: change each line in `internal/router/router.go` that mounts an HTMX handler to `spa.FallbackHandler()` and remove the corresponding handler import. Reversible by reverting the commit.
