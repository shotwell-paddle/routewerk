# SPA rebuild — cleanup audit

After the Phase-2 rebuild plus the parity sweep in PRs #77–#93, the
SvelteKit shell at `/app/*` covers nearly the entire HTMX climber +
staff surface. This doc tracks each HTMX route as one of:

- **mirrored** — SPA at `/app/*` covers it; HTMX template + handler can
  be deleted in a follow-up cleanup PR.
- **partial** — SPA covers most of the feature, gap noted.
- **unique** — still HTMX-only; either intentionally so, awaits backend
  work, or is a server-rendered artifact.

Nothing here is destructive — the actual HTMX template / handler
deletions are a separate PR once the user signs off on the swap.

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
> presets, #93 activity feed.

---

## Status by HTMX surface

### Auth (HTMX-only by design)

| HTMX route | Status | Notes |
|---|---|---|
| `GET /login` + `POST /login` | unique | Password auth; SPA `/sign-in` is magic-link only. |
| `GET /register` + `POST /register` | unique | New account creation. |
| `GET /verify-magic` | server-rendered | Magic-link callback; cannot be SPA. |
| `GET /setup` + `POST /setup` | unique | First-run org bootstrap. |
| `GET /join-gym` etc. | unique | Climber onboarding. |
| `POST /logout` | server-rendered | Cookie clear + redirect. |

### Climber-facing

| HTMX route | Status | SPA equivalent |
|---|---|---|
| `GET /routes` | mirrored | `/app/routes` |
| `GET /archive` | mirrored | `/app/archive` (#84) |
| `GET /routes/{id}` | mirrored | `/app/routes/[id]` — log ascent (#68), rate, photo upload (#81), community tags (#83), difficulty consensus (#83) |
| `POST /routes/{id}/ascent` | mirrored | `POST /api/v1/locations/{loc}/routes/{id}/ascent` |
| `POST /routes/{id}/rate` | mirrored | `POST /api/v1/locations/{loc}/routes/{id}/rate` |
| `POST /routes/{id}/difficulty` | mirrored | `POST /api/v1/locations/{loc}/routes/{id}/difficulty` (#83) |
| `POST /routes/{id}/tags`, `/tags/remove`, `/tags/delete` | mirrored | `/api/v1/locations/{loc}/routes/{id}/tags` (#83) |
| `POST /routes/{id}/photos`, `/photos/{id}/delete` | mirrored | `/api/v1/locations/{loc}/routes/{id}/photos` (#81) |
| `GET /routes/{id}/card/...` | server-rendered | Same endpoint serves both surfaces |
| `GET /routes/{id}/ascents-feed` | unique | HTMX live ticker; the SPA's route detail polls `listRouteAscents` |
| `GET /profile`, `/profile/settings`, `POST /profile/*` | mirrored | `/app/profile`, `/app/settings` |
| `GET /profile/ticks/{id}/edit`, `POST .../{id}`, `/delete` | mirrored | inline edit + delete on `/app/profile` (#86) |
| `GET /notifications`, `POST .../*` | mirrored | `/app/notifications` (#69) |
| `GET /quests`, `/quests/mine`, `/quests/{id}` | mirrored | `/app/quests/*` |
| `POST /quests/{id}/{start,log,abandon}` | mirrored | JSON helpers |
| `POST /quests/{id}/complete` | unique | Manual mark-complete; auto-complete fires server-side |
| `GET /quests/badges` | mirrored | `/app/quests/badges` (#84) |
| `GET /quests/activity` | mirrored | `/app/quests/activity` (#93) |

### Setter / staff-facing

| HTMX route | Status | SPA equivalent |
|---|---|---|
| `GET /dashboard` | mirrored | `/app` — stats + activity feed (#70) + wall-by-route grid (#79) |
| `GET /routes/manage`, `/routes/new`, etc. | mirrored | `/app/routes/*` — gym-aware pickers + auto strip-date (#78) |
| `GET /walls/*`, archive/unarchive/delete | mirrored | `/app/walls/*` (#71) |
| `GET /sessions/*`, full lifecycle | mirrored | `/app/sessions/*` (#72) |
| `POST /sessions/{id}/publish` | mirrored | one-shot publish on `/app/sessions/[id]` (#87) |
| `GET /sessions/{id}/photos`, `POST /sessions/{id}/routes/...` | unique | Per-session route ops; needs backend port |
| `GET /card-batches/*` | mirrored | `/app/card-batches/*` |
| `POST /card-batches/{id}/edit` | mirrored | inline edit on `/app/card-batches/[id]` (#91) |
| `GET /card-batches/{id}/{cutlines.dxf,preview.png}` | server-rendered | Downloadable artifacts |
| `GET /settings`, gym settings + circuits + hold-colors | mirrored | `/app/settings/gym` (#73) — head_setter+ writes (#82) |
| `POST /settings/palette-preset` | mirrored | one-click presets in `/app/settings/gym` (#92) |
| `POST /settings/progressions-toggle` | mirrored | `/app/settings/gym` toggle card (#89) |
| `GET /settings/team` | mirrored | `/app/team` |
| `POST /switch-view-as` | mirrored | sidebar view-as bar (#80) + page gates (#82) |
| `GET /settings/organization`, gym CRUD | mirrored | `/app/settings/org` (#74) |
| `GET /settings/organization/team` | mirrored | "Whole org" toggle on `/app/team` (#88) |
| `GET /settings/progressions`, domain/badge CRUD | partial | `/app/settings/progressions` (#75); **quest create/edit form still HTMX** |
| `GET /settings/playbook` + edits | unique | Setter playbook editor — new domain, not yet in SPA |
| `GET /admin/*` (health, metrics) | unique | App-admin observability; intentionally server-rendered |

### Competitions

`/app/competitions/*` (renamed from `/staff/comp/*` in #78) and
`/comp/[slug]` are SPA-only — no HTMX cousins.

---

## Remaining HTMX-only items (full list)

These are the only HTMX surfaces that don't have a `/app/*` equivalent:

- **Auth flows** (`/login`, `/register`, `/verify-magic`, `/setup`, `/join-gym`) — intentionally server-rendered; the SPA's magic-link `/sign-in` is the new account-recovery path.
- **`/routes/{id}/ascents-feed`** — live HTMX ticker. The SPA route detail already shows recent ascents via `listRouteAscents`; a polling/SSE upgrade is on the wishlist but not blocking.
- **`POST /quests/{id}/complete`** — manual mark-complete. Auto-complete fires server-side; the manual path is a recovery mechanism rarely used.
- **`GET /sessions/{id}/photos` + `POST /sessions/{id}/routes/*`** — session-scoped photo gallery + per-session route edits. Needs new JSON endpoints; **stretch item**.
- **Quest create/edit form** (`POST /settings/progressions/quests/{id}/...`) — rich form (completion criteria, target count, availability window, route tag filter). The SPA progressions admin links out to HTMX for now. **Stretch item; deserves a dedicated PR.**
- **Playbook editor** (`GET /settings/playbook` + child edits) — setter checklist templates. New domain, not in SPA. **Stretch item.**
- **`/admin/health`, `/admin/metrics`** — app-admin observability. Server-rendered for immediate dashboards; not planned for SPA.

---

## Recommendation: when to swap `/app/*` to root

After the items above either land or are explicitly deferred:

1. Delete the HTMX templates + handlers for the surfaces marked **mirrored**. Single deletion PR for review-ability.
2. Swap each HTMX router mount in `internal/router/router.go` to point at the SPA fallback — a one-line change per row, fully reversible.
3. For surfaces that stay HTMX (auth, `/admin/*`, the few unique items), keep their mounts intact.
4. The HTMX-side templates that share partials (`partials/route-card.html`, etc.) are still load-bearing for server-rendered cards and the magic-link callback — don't touch those without grepping.

The actual swap-to-root is mechanical once the deletes land. Reversible by reverting the commit if the user spots a regression.
