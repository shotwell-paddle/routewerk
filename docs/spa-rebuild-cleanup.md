# SPA rebuild — cleanup audit

After the Phase-2 rebuild ran end-to-end, plus the gym/team/quests/etc.
follow-up PRs (#66 → #75), the SvelteKit shell at `/app/*` covers most of
the existing HTMX surface. This doc inventories every HTMX route as one
of:

- **mirrored** — SPA at `/app/*` covers it; safe to delete the HTMX
  template + handler in a follow-up cleanup PR.
- **partial** — SPA covers most of the feature, gap noted.
- **unique** — still the only home for that feature; either needs
  backend work to enable a SPA migration, is intentionally HTMX
  forever, or is a server-rendered artifact.

The recommendation at the bottom is what to delete now vs. defer.
Nothing here is destructive — the actual deletes come in a separate PR
once you sign off.

> SPA shell + features shipped in PRs:
> #50–#53 (Phase 1h staff comp), #54 head_setter authz,
> #55 2.1 shell + palette, #56 walls, #57 routes browser,
> #58 routes detail, #59 sessions list, #60 card batches,
> #61 profile + settings, #62 team, #63 quests browse,
> #64 audit doc + accent sweep, #65 role resolution fix,
> #66 palette revert + sidebar match, #67 self-demotion guard,
> #68 climber log+rate, #69 notifications, #70 dashboard stats,
> #71 wall archive, #72 session lifecycle (assignments + strip + checklist),
> #73 gym settings, #74 org admin, #75 progressions admin.

---

## Status by HTMX surface

### Auth

| HTMX route | Status | SPA equivalent | Notes |
|---|---|---|---|
| `GET /` | redirect | — | redirects logged-in users to `/dashboard`; once `/app/*` swaps to root this becomes the SPA shell |
| `GET /login` + `POST /login` | unique | — | password auth; SPA `/sign-in` is magic-link only |
| `GET /register` + `POST /register` | unique | — | new account creation |
| `GET /verify-magic` | server-rendered | — | callback for magic-link emails. Cannot become SPA. |
| `GET /setup` + `POST /setup` | unique | — | first-run org bootstrap |
| `GET /join-gym` etc. | unique | — | climber-onboarding flow |
| `POST /logout` | server-rendered | — | clears cookie + redirects |

### Climber-facing

| HTMX route | Status | SPA equivalent | Notes |
|---|---|---|---|
| `GET /routes` | partial | `/app/routes` | SPA `/app/routes` is staff-flavored (status filters, manage actions). Climber view (community tags, ascent CTA) lives within it via the same browse page. **Could be deleted; the SPA browser works for climbers too — just verify on dev first.** |
| `GET /archive` | unique | — | climber archive view of stripped routes |
| `GET /routes/{id}` | partial | `/app/routes/{id}` | covered ✓ — log ascent + rate live there (#68). Community tags + photo upload still HTMX-only. |
| `POST /routes/{id}/ascent` | mirrored | `/api/v1/locations/{loc}/routes/{id}/ascent` | ✓ |
| `POST /routes/{id}/rate` | mirrored | `/api/v1/locations/{loc}/routes/{id}/rate` | ✓ |
| `GET /routes/{id}/card/{print|share}.{png,pdf}` | server-rendered | — | rendered server-side, not a UI surface |
| `POST /routes/{id}/difficulty`, `/tags`, `/photos`, `/photos/{id}/delete` | unique | — | climber-side route interactions, not migrated |
| `GET /routes/{id}/ascents-feed` | unique | — | live HTMX feed partial |
| `GET /profile`, `/profile/ticks`, `/profile/settings`, `POST /profile/*` | mirrored | `/app/profile`, `/app/settings` | ✓ |
| `GET /profile/ticks/{id}/edit`, `POST /profile/ticks/{id}`, `/delete` | unique | — | per-tick edit/delete |
| `GET /notifications`, `/notifications/badge` | mirrored | `/app/notifications` + sidebar badge | ✓ (#69) |
| `POST /notifications/...` | mirrored | `/api/v1/me/notifications/{id}/read` etc | ✓ |
| `GET /quests`, `/quests/mine`, `/quests/{id}` | mirrored | `/app/quests` + `/app/quests/{id}` | ✓ |
| `POST /quests/{id}/start`, `/log`, `/abandon` | mirrored | JSON helpers | ✓ |
| `POST /quests/{id}/complete` | unique | — | manual mark-complete (auto-complete fires via service) |
| `GET /quests/badges` | unique | — | badge showcase grid |
| `GET /quests/activity` | unique | — | quest activity feed |

### Setter / staff-facing

| HTMX route | Status | SPA equivalent | Notes |
|---|---|---|---|
| `GET /dashboard` | mirrored | `/app` | stats + activity feed (#70). Dashboard's HTMX-side wall-by-route grid not in SPA but quick-action cards link out. |
| `GET /routes/manage`, `/routes/new`, `POST /routes/new`, `/routes/{id}/edit`, `POST .../edit`, `POST .../status` | mirrored | `/app/routes/*` | ✓ |
| `GET /walls` etc + `POST /walls/{id}/{archive,unarchive,delete}` | mirrored | `/app/walls/*` | ✓ — archive added in #71 |
| `GET /sessions/*`, `POST /sessions/{id}/{assign,strip,checklist,delete}` | mirrored | `/app/sessions/*` | ✓ — full lifecycle in #72 except `POST /sessions/{id}/publish` (multi-step strip+publish) and `GET /sessions/{id}/photos` |
| `POST /sessions/{id}/publish` | unique | — | combines strip-targets archive + publish-draft-routes + status flip; SPA links out |
| `GET /sessions/{id}/photos`, `POST /sessions/{id}/routes/...` | unique | — | session photos + per-session route ops |
| `GET /card-batches` etc | mirrored | `/app/card-batches/*` | ✓ |
| `GET /card-batches/{id}/edit`, `POST .../edit` | unique | — | SPA has no edit form (theme + cutter are pickable on create only) |
| `GET /card-batches/{id}/cutlines.dxf`, `/preview.png` | server-rendered | — | downloadable artifacts |
| `GET /settings` (gym), `POST /settings/circuits/...`, `/hold-colors/...`, `/palette-preset` | mirrored | `/app/settings/gym` | ✓ (#73) — palette presets not in SPA editor (editing colors directly works) |
| `GET /settings/team`, `POST /settings/team/{id}/role` | mirrored | `/app/team` | ✓ |
| `GET /settings/playbook` + edit endpoints | unique | — | setter playbook editor |
| `POST /settings/progressions-toggle` | unique | — | feature flag toggle for the progressions surface |
| `GET /settings/organization` + child writes | mirrored | `/app/settings/org` | ✓ (#74) |
| `GET /settings/organization/gyms/...` | mirrored | `/app/settings/org` | gym CRUD covered ✓ |
| `GET /settings/organization/team` | partial | `/app/team` | location-scoped team only; org-wide team list is HTMX-only |
| `GET /settings/progressions` + child editors | partial | `/app/settings/progressions` | ✓ list + deactivate + duplicate + domain/badge create+delete (#75). **Quest create/edit form still links to HTMX** because the form is rich (completion criteria + target count + availability window + route tag filter). Porting deserves a dedicated PR. |
| `GET /admin/health`, `/admin/metrics` | unique | — | app-admin observability |

### Competitions

The `/comp/*` and `/staff/comp/*` SPA surfaces (Phase 1g + 1h) have always
been SPA-only. No HTMX equivalents to delete.

---

## Recommendation: delete now (HTMX surfaces fully mirrored)

These are safe to delete — the SPA covers them with full parity. Deletion
should be a single PR for review-ability. Be careful: some HTMX templates
share partials with the climber-facing route detail; check imports.

Routes:
- `/profile`, `/profile/ticks`, `/profile/settings`, `POST /profile/settings`, `POST /profile/password`
- `/notifications`, `/notifications/badge`, `POST /notifications/...`
- `/quests`, `/quests/mine`, `/quests/{id}`, `POST /quests/{id}/{start,log,abandon}`
- `/walls`, `/walls/new`, `POST /walls/new`, `/walls/{id}/edit`, `/walls/{id}/{archive,unarchive,delete}`
- `/routes/new`, `/routes/new/fields`, `POST /routes/new`, `/routes/{id}/edit`, `POST .../edit`, `POST .../status`
- `/sessions/new`, `POST /sessions/new`, `/sessions/{id}/edit`, `POST .../edit`, `POST /sessions/{id}/delete`, `/assignments`, `/strip*`, `/checklist*`
- `/card-batches`, `/card-batches/new`, `/card-batches/{id}`, `POST /card-batches/{id}/delete`
- `/settings/team`, `POST /settings/team/{id}/role`
- `/settings` (gym) + `/settings/circuits/*` + `/settings/hold-colors/*`
- `/settings/organization`, `POST /settings/organization`, `/settings/organization/gyms/{new,{id}/edit}`

Templates that go with them (`web/templates/`):
`climber/profile.html`, `climber/profile-settings.html`,
`climber/notifications.html`, `climber/quests.html`, `climber/quest-detail.html`,
`climber/my-quests.html`,
`setter/walls.html`, `setter/wall-form.html`, `setter/wall-detail.html`,
`setter/route-form.html`, `setter/route-manage.html`,
`setter/sessions.html`, `setter/session-detail.html`, `setter/session-form.html`,
`setter/team.html`, `setter/card-batches.html`, `setter/card-batch-form.html`,
`setter/card-batch-detail.html`, `setter/settings.html`, `setter/org-settings.html`,
`setter/org-team.html`, `setter/gym-edit.html`, `setter/gym-new.html`.

> ⚠ Don't delete `partials/route-card.html` — still used by the climber
> route detail and dashboard wall-by-route grid which haven't migrated.

## Recommendation: keep for now (still unique)

- All auth flows (`/login`, `/register`, `/verify-magic`, `/setup`, `/join-gym`)
- Climber route detail interactions still HTMX-only: community tags + photo upload (`POST /routes/{id}/{tags,photos}` etc)
- `/archive` (climber stripped-routes view)
- `/routes/{id}/ascents-feed` (live ascent ticker)
- `/profile/ticks/{id}/edit`, `POST .../{id}`, `POST .../delete` (per-tick edit)
- `/quests/badges`, `/quests/activity`, `POST /quests/{id}/complete`
- Setter dashboard's wall-by-route grid (the SPA `/app` shows stats + activity feed but not the per-wall route grid)
- Session photos + lifecycle multi-step (`POST /sessions/{id}/publish`, `GET /sessions/{id}/{complete,photos}`, `POST /sessions/{id}/routes/...`)
- Card batch edit + cutlines + preview
- Gym settings palette presets (`POST /settings/palette-preset`)
- Setter playbook + progressions toggle (`/settings/playbook`, `POST /settings/progressions-toggle`)
- Org-wide team list (`/settings/organization/team`)
- Progressions admin: quest create + edit form (`/settings/progressions/quests/new` etc)
- Admin observability (`/admin/*`)
- Server-rendered artifacts (route cards, card batch PDFs, magic-link callback, logout)

## Suggested follow-up PRs (in roughly increasing complexity)

1. **Cleanup PR** — delete the templates + handlers from "delete now" above. Net diff: hundreds of lines removed.
2. **Climber route interactions parity** — community tags + photo upload on `/app/routes/[id]`. Needs JSON tag + photo endpoints (likely already exist; check).
3. **Setter dashboard wall-by-route grid** — derive from `listWalls` + `listRoutes(active)` client-side, append to `/app`.
4. **Session photos + publish flow** — the multi-step publish (strip + draft publish + status flip) ported to a dedicated SPA flow.
5. **Quest create/edit form** in the SPA — finish the progressions admin migration.
6. **Org-wide team management** — `/app/team` becomes location- AND org-scoped depending on context.

## Recommendation: when to swap `/app/*` to root

Hold off until:
1. The user has confirmed the SPA renders correctly with their data on `routewerk-dev` for all the surfaces above.
2. The "delete now" list above has actually been deleted (otherwise the swap is a routing trap — stale links to deleted templates would 404).
3. A redirect plan exists for the surfaces that still live at HTMX paths (e.g. `/profile` → `/app/profile`, `/walls` → `/app/walls`).

The router swap itself is mechanical: change each line in
`internal/router/router.go` that mounts an HTMX handler to
`spa.FallbackHandler()` and remove the corresponding handler import.
Reversible by reverting the commit.
