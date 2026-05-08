// Minimal typed API client. Wraps fetch with cookie credentials enabled
// (so the SPA's session auth flows through) and surfaces typed
// request/response payloads from the generated `paths` map.
//
// Phase 1e ships only the magic-link request endpoint as the first
// caller — proving the spec → openapi-typescript → SPA loop works.
// Phase 1f's API handlers will be wired up here as they land.
//
// All API access in the SPA goes through this module; no raw `fetch`
// calls in components.

import type { components, paths } from './types';

/** Convenient alias for the generated component schemas. */
export type Schemas = components['schemas'];

/** Typed shapes by name (e.g. `Schemas['Competition']`). */
export type Competition = Schemas['Competition'];
export type CompetitionCreate = Schemas['CompetitionCreate'];
export type CompetitionUpdate = Schemas['CompetitionUpdate'];
export type CompetitionStatus = Schemas['CompetitionStatus'];
export type CompetitionFormat = Schemas['CompetitionFormat'];
export type CompetitionEvent = Schemas['CompetitionEvent'];
export type CompetitionCategory = Schemas['CompetitionCategory'];
export type CompetitionProblem = Schemas['CompetitionProblem'];
export type CompetitionRegistration = Schemas['CompetitionRegistration'];
export type RegistrationCreate = Schemas['RegistrationCreate'];
export type EventCreate = Schemas['EventCreate'];
export type EventUpdate = Schemas['EventUpdate'];
export type CategoryCreate = Schemas['CategoryCreate'];
export type ProblemCreate = Schemas['ProblemCreate'];
export type ProblemUpdate = Schemas['ProblemUpdate'];
export type Aggregation = Schemas['Aggregation'];
export type MagicLinkRequest = Schemas['MagicLinkRequest'];
export type ApiError = Schemas['Error'];

// Action endpoint types (Phase 1g.2 onward).
export type ActionType = Schemas['ActionType'];
export type ActionItem = Schemas['ActionItem'];
export type ActionsRequest = Schemas['ActionsRequest'];
export type ActionsResponse = Schemas['ActionsResponse'];
export type AttemptState = Schemas['AttemptState'];
export type ActionRejectedReason = Schemas['ActionRejected']['reason'];

// Leaderboard types (Phase 1g.3).
export type Leaderboard = Schemas['Leaderboard'];
export type LeaderboardEntry = Schemas['LeaderboardEntry'];

/**
 * ApiClientError is thrown for any non-2xx response. The caller can
 * catch and inspect `status` + `body` to render an appropriate message.
 */
export class ApiClientError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiError | string,
    message?: string,
  ) {
    super(message ?? (typeof body === 'object' ? body.error : String(body)));
    this.name = 'ApiClientError';
  }
}

/**
 * Base URL for API calls. In dev the SPA dev server proxies /api → :8080
 * (see vite.config.ts). In production the SPA is served same-origin
 * from the Go binary, so a relative path works.
 */
const API_BASE = '/api/v1';

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  body?: unknown;
  /** Optional AbortSignal to cancel the request (e.g. on component unmount). */
  signal?: AbortSignal;
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers: HeadersInit = { Accept: 'application/json' };
  let body: BodyInit | undefined;
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(opts.body);
  }

  const res = await fetch(`${API_BASE}${path}`, {
    method: opts.method ?? 'GET',
    headers,
    body,
    credentials: 'same-origin',
    signal: opts.signal,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  let payload: unknown;
  const text = await res.text();
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }

  if (!res.ok) {
    throw new ApiClientError(res.status, payload as ApiError | string);
  }
  return payload as T;
}

// ── Endpoints ──────────────────────────────────────────────

/**
 * POST /auth/magic/request — request a magic-link sign-in email.
 * Always resolves with `{ ok: true }` regardless of whether the email
 * is registered; do not infer account existence from the response.
 */
export async function requestMagicLink(
  body: MagicLinkRequest,
  signal?: AbortSignal,
): Promise<paths['/auth/magic/request']['post']['responses']['202']['content']['application/json']> {
  return request('/auth/magic/request', { method: 'POST', body, signal });
}

// ── /me (not yet in spec — added directly while we expand the spec
//      to cover existing routes piecemeal in later phases) ──────────

export interface MembershipShape {
  id: string;
  user_id: string;
  org_id: string;
  location_id?: string | null;
  role: string;
  specialties?: string[];
  created_at: string;
  updated_at: string;
}

export interface UserShape {
  id: string;
  email: string;
  display_name: string;
  avatar_url?: string | null;
  bio?: string | null;
  is_app_admin: boolean;
  created_at: string;
  updated_at: string;
}

export interface MeResponse {
  user: UserShape;
  memberships: MembershipShape[];
  /**
   * Active view-as override role from the `_rw_view_as` cookie, or "" if
   * none. Lets staff demo lower-role views without re-authenticating.
   * Mirrors the HTMX sidebar's view-as bar.
   */
  view_as_role?: string;
}

// Location is not yet in the OpenAPI spec (existing HTMX/API surface
// hasn't been migrated). Hand-written shape mirrors what the server
// returns from GET /api/v1/locations/{id}.
export interface LocationShape {
  id: string;
  org_id: string;
  name: string;
  slug: string;
  timezone: string;
  address?: string | null;
  website_url?: string | null;
  phone?: string | null;
  custom_domain?: string | null;
  /**
   * Feature flag that gates the quests / progressions surface at this
   * location. The HTMX side checks the same field via Location.ProgressionsEnabled
   * (see internal/handler/web/progressions_climber.go::progressionsGated).
   */
  progressions_enabled?: boolean;
  created_at: string;
  updated_at: string;
}

/**
 * GET /api/v1/locations/{id} — returns location metadata. Caller must
 * be a member of the location's org (server-side enforced). Resolves
 * with `null` on 404 (e.g. caller has no membership).
 */
export async function getLocation(id: string, signal?: AbortSignal): Promise<LocationShape | null> {
  try {
    return await request<LocationShape>(`/locations/${id}`, { signal });
  } catch (err) {
    if (err instanceof ApiClientError && (err.status === 404 || err.status === 403)) {
      return null;
    }
    throw err;
  }
}

/**
 * GET /api/v1/orgs/{orgId}/locations — every location in an org. Any
 * member of the org can list. The SPA uses this to discover which
 * locations an org-wide membership (location_id null) gives access to,
 * since /me only returns membership rows, not the org's full footprint.
 */
export async function listOrgLocations(
  orgId: string,
  signal?: AbortSignal,
): Promise<LocationShape[]> {
  return request(`/orgs/${orgId}/locations`, { signal });
}

// ── Location settings (gym-level) ─────────────────────────
//
// Hand-written shapes mirroring internal/model/settings.go::LocationSettings.
// Settings live in a single JSONB column on locations and are read+written
// as one struct.

export interface CircuitColor {
  name: string;
  hex: string;
  sort_order: number;
}

export interface HoldColor {
  name: string;
  hex: string;
}

export interface GradingSettings {
  boulder_method: string;
  route_grade_format: string;
  show_grades_on_circuit: boolean;
  v_scale_range?: string[];
  yds_range?: string[];
}

export interface CircuitSettings {
  colors: CircuitColor[];
}

export interface HoldColorSettings {
  colors: HoldColor[];
}

export interface DisplaySettings {
  show_setter_name: boolean;
  show_route_age: boolean;
  show_difficulty_consensus: boolean;
  default_strip_age_days: number;
}

export interface SessionSettings {
  default_playbook_enabled: boolean;
  require_route_photo: boolean;
}

export interface LocationSettingsShape {
  grading: GradingSettings;
  circuits: CircuitSettings;
  hold_colors: HoldColorSettings;
  display: DisplaySettings;
  sessions: SessionSettings;
}

/** GET /locations/{locationId}/settings — gym_manager+. */
export async function getLocationSettings(
  locationId: string,
  signal?: AbortSignal,
): Promise<LocationSettingsShape> {
  return request(`/locations/${locationId}/settings`, { signal });
}

/**
 * POST /locations/{locationId}/progressions-toggle — gym_manager+ flips
 * the climber-facing progressions feature flag. Mirrors the HTMX
 * /settings/progressions-toggle endpoint.
 */
export async function setLocationProgressions(
  locationId: string,
  enabled: boolean,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/progressions-toggle`, {
    method: 'POST',
    body: { enabled },
    signal,
  });
}

export interface PalettePresetSwatch {
  name: string;
  hex: string;
}

export interface PalettePresetEntry {
  name: string;
  display_name: string;
  description: string;
  circuits: PalettePresetSwatch[];
}

/**
 * GET /locations/{locationId}/settings/palette-presets — setter+.
 * Static catalog of named palette presets the SPA can render as
 * one-click apply buttons.
 */
export async function listPalettePresets(
  locationId: string,
  signal?: AbortSignal,
): Promise<PalettePresetEntry[]> {
  return request(`/locations/${locationId}/settings/palette-presets`, { signal });
}

/**
 * POST /locations/{locationId}/settings/palette-preset — head_setter+.
 * Replaces circuits + hold-color lists with the named preset in one
 * shot. Returns the post-apply settings so the SPA can refresh inline.
 */
export async function applyPalettePreset(
  locationId: string,
  preset: string,
  signal?: AbortSignal,
): Promise<LocationSettingsShape> {
  return request(`/locations/${locationId}/settings/palette-preset`, {
    method: 'POST',
    body: { preset },
    signal,
  });
}

// ── Route distribution (server-aggregated) ────────────────

export interface RouteDistributionBucket {
  route_type: string;
  grading_system: string;
  grade: string;
  circuit_color?: string | null; // hex; only set when grading_system === 'circuit'
  count: number;
}

/**
 * GET /locations/{locationId}/route-distribution — setter+.
 *
 * Server-side GROUP BY over active routes. Replaces the dashboard's
 * old pattern of fetching all 500 active routes just to count them
 * client-side; this returns ~30 rows regardless of the gym's route
 * count. The dashboard derives both grade and circuit charts from the
 * same response — `grading_system === 'circuit'` rows feed the
 * circuit chart, the rest feed the grade chart.
 */
export async function listRouteDistribution(
  locationId: string,
  signal?: AbortSignal,
): Promise<RouteDistributionBucket[]> {
  return request(`/locations/${locationId}/route-distribution`, { signal });
}

/** PUT /locations/{locationId}/settings — gym_manager+. Replaces the full struct. */
export async function updateLocationSettings(
  locationId: string,
  body: LocationSettingsShape,
  signal?: AbortSignal,
): Promise<LocationSettingsShape> {
  return request(`/locations/${locationId}/settings`, { method: 'PUT', body, signal });
}

// ── Route distribution targets ────────────────────────────

export type DistributionTargetType = 'boulder' | 'route' | 'circuit';

export interface DistributionTarget {
  id: string;
  location_id: string;
  route_type: DistributionTargetType;
  grade: string;
  target_count: number;
  created_at: string;
  updated_at: string;
}

export interface DistributionTargetWrite {
  route_type: DistributionTargetType;
  grade: string;
  target_count: number;
}

/**
 * GET /locations/{locationId}/distribution-targets — setter+.
 * Returns the per-(route_type, grade) goals the dashboard charts
 * overlay against actual route counts.
 */
export async function listDistributionTargets(
  locationId: string,
  signal?: AbortSignal,
): Promise<DistributionTarget[]> {
  return request(`/locations/${locationId}/distribution-targets`, { signal });
}

/**
 * PUT /locations/{locationId}/distribution-targets — head_setter+.
 * Replace-all: wipes the existing target set and inserts the
 * caller-supplied entries in one transaction. Entries with
 * target_count <= 0 are dropped server-side.
 */
export async function replaceDistributionTargets(
  locationId: string,
  targets: DistributionTargetWrite[],
  signal?: AbortSignal,
): Promise<DistributionTarget[]> {
  return request(`/locations/${locationId}/distribution-targets`, {
    method: 'PUT',
    body: { targets },
    signal,
  });
}

// ── Organizations (org admin surface) ─────────────────────

export interface OrgShape {
  id: string;
  name: string;
  slug: string;
  logo_url?: string | null;
  created_at: string;
  updated_at: string;
}

/** GET /api/v1/orgs — orgs the caller is a member of. */
export async function listMyOrgs(signal?: AbortSignal): Promise<OrgShape[]> {
  return request('/orgs', { signal });
}

/** GET /api/v1/orgs/{orgId} — any member of the org. */
export async function getOrg(orgId: string, signal?: AbortSignal): Promise<OrgShape> {
  return request(`/orgs/${orgId}`, { signal });
}

export interface OrgUpdateShape {
  name: string;
  slug: string;
  logo_url?: string | null;
}

/** PUT /api/v1/orgs/{orgId} — org_admin only. */
export async function updateOrg(
  orgId: string,
  body: OrgUpdateShape,
  signal?: AbortSignal,
): Promise<OrgShape> {
  return request(`/orgs/${orgId}`, { method: 'PUT', body, signal });
}

export interface LocationCreateShape {
  name: string;
  slug?: string;
  address?: string | null;
  timezone?: string;
  website_url?: string | null;
  phone?: string | null;
  day_pass_info?: string | null;
  waiver_url?: string | null;
  allow_shared_setters?: boolean;
}

/** POST /api/v1/orgs/{orgId}/locations — org_admin only. */
export async function createLocation(
  orgId: string,
  body: LocationCreateShape,
  signal?: AbortSignal,
): Promise<LocationShape> {
  return request(`/orgs/${orgId}/locations`, { method: 'POST', body, signal });
}

/** PUT /api/v1/locations/{id} — org_admin only. */
export async function updateLocation(
  locationId: string,
  body: LocationCreateShape,
  signal?: AbortSignal,
): Promise<LocationShape> {
  return request(`/locations/${locationId}`, { method: 'PUT', body, signal });
}

// ── Walls (Phase 2.2) ─────────────────────────────────────
//
// Hand-written shapes — wall handlers haven't been migrated to the
// OpenAPI spec yet. Mirror `internal/model/models.go::Wall` and
// `internal/handler/wall.go::createWallRequest`.

export type WallType = 'boulder' | 'route';

export interface WallShape {
  id: string;
  location_id: string;
  name: string;
  wall_type: WallType;
  angle?: string | null;
  height_meters?: number | null;
  num_anchors?: number | null;
  surface_type?: string | null;
  sort_order: number;
  map_x?: number | null;
  map_y?: number | null;
  map_width?: number | null;
  map_height?: number | null;
  created_at: string;
  updated_at: string;
  archived_at?: string | null;
}

export interface WallWriteShape {
  name: string;
  wall_type: WallType;
  angle?: string | null;
  height_meters?: number | null;
  num_anchors?: number | null;
  surface_type?: string | null;
  sort_order: number;
  map_x?: number | null;
  map_y?: number | null;
  map_width?: number | null;
  map_height?: number | null;
}

/**
 * GET /locations/{locationId}/walls — ascending by sort_order. By default
 * returns only non-archived walls (matches climber-side expectations).
 * Pass `includeArchived: true` to also see archived walls (staff manage
 * page).
 */
export async function listWalls(
  locationId: string,
  opts: { includeArchived?: boolean } = {},
  signal?: AbortSignal,
): Promise<WallShape[]> {
  const qs = opts.includeArchived ? '?include_archived=true' : '';
  return request(`/locations/${locationId}/walls${qs}`, { signal });
}

/** GET /locations/{locationId}/walls/{wallId} */
export async function getWall(
  locationId: string,
  wallId: string,
  signal?: AbortSignal,
): Promise<WallShape> {
  return request(`/locations/${locationId}/walls/${wallId}`, { signal });
}

/** POST /locations/{locationId}/walls — setter+ at the location. */
export async function createWall(
  locationId: string,
  body: WallWriteShape,
  signal?: AbortSignal,
): Promise<WallShape> {
  return request(`/locations/${locationId}/walls`, { method: 'POST', body, signal });
}

/** PUT /locations/{locationId}/walls/{wallId} — setter+ at the location. */
export async function updateWall(
  locationId: string,
  wallId: string,
  body: WallWriteShape,
  signal?: AbortSignal,
): Promise<WallShape> {
  return request(`/locations/${locationId}/walls/${wallId}`, { method: 'PUT', body, signal });
}

/** DELETE /locations/{locationId}/walls/{wallId} — head_setter+ at the location. */
export async function deleteWall(
  locationId: string,
  wallId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/walls/${wallId}`, { method: 'DELETE', signal });
}

/** POST /locations/{locationId}/walls/{wallId}/archive — head_setter+. */
export async function archiveWall(
  locationId: string,
  wallId: string,
  signal?: AbortSignal,
): Promise<WallShape> {
  return request(`/locations/${locationId}/walls/${wallId}/archive`, {
    method: 'POST',
    signal,
  });
}

/** POST /locations/{locationId}/walls/{wallId}/unarchive — head_setter+. */
export async function unarchiveWall(
  locationId: string,
  wallId: string,
  signal?: AbortSignal,
): Promise<WallShape> {
  return request(`/locations/${locationId}/walls/${wallId}/unarchive`, {
    method: 'POST',
    signal,
  });
}

// ── Routes (Phase 2.3) ────────────────────────────────────
//
// Hand-written shapes — routes aren't in the OpenAPI spec yet.
// Mirror `internal/model/models.go::Route` and `RouteHandler.Create`.

export type RouteType = 'boulder' | 'route';
export type RouteStatus = 'active' | 'flagged' | 'archived';

export interface RouteShape {
  id: string;
  location_id: string;
  wall_id: string;
  setter_id?: string | null;
  route_type: RouteType;
  status: RouteStatus;
  grading_system: string;
  grade: string;
  grade_low?: string | null;
  grade_high?: string | null;
  circuit_color?: string | null;
  name?: string | null;
  color: string;
  color_name?: string | null;
  description?: string | null;
  photo_url?: string | null;
  date_set: string;
  projected_strip_date?: string | null;
  date_stripped?: string | null;
  avg_rating: number;
  rating_count: number;
  ascent_count: number;
  attempt_count: number;
  session_id?: string | null;
  tags?: TagShape[];
  created_at: string;
  updated_at: string;
}

export interface TagShape {
  id: string;
  org_id: string;
  category: string;
  name: string;
}

/** GET /api/v1/orgs/{orgId}/tags — any org member. */
export async function listOrgTags(
  orgId: string,
  signal?: AbortSignal,
): Promise<TagShape[]> {
  return request(`/orgs/${orgId}/tags`, { signal });
}

export interface RouteListResponse {
  routes: RouteShape[];
  total: number;
  limit: number;
  offset: number;
}

export interface RouteListFilters {
  wall_id?: string;
  status?: RouteStatus;
  route_type?: RouteType;
  grade?: string;
  setter_id?: string;
  limit?: number;
  offset?: number;
}

/** GET /locations/{locationId}/routes — paginated, filtered. */
export async function listRoutes(
  locationId: string,
  filters: RouteListFilters = {},
  signal?: AbortSignal,
): Promise<RouteListResponse> {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(filters)) {
    if (v != null && v !== '') qs.set(k, String(v));
  }
  const suffix = qs.toString() ? `?${qs}` : '';
  return request(`/locations/${locationId}/routes${suffix}`, { signal });
}

/** GET /locations/{locationId}/routes/{routeId} */
export async function getRoute(
  locationId: string,
  routeId: string,
  signal?: AbortSignal,
): Promise<RouteShape> {
  return request(`/locations/${locationId}/routes/${routeId}`, { signal });
}

export interface RouteWriteShape {
  wall_id: string;
  route_type: RouteType;
  grading_system: string;
  grade: string;
  color: string;
  grade_low?: string | null;
  grade_high?: string | null;
  circuit_color?: string | null;
  name?: string | null;
  description?: string | null;
  photo_url?: string | null;
  /** ISO date (YYYY-MM-DD). */
  date_set?: string | null;
  projected_strip_date?: string | null;
  tag_ids?: string[];
}

/** POST /locations/{locationId}/routes — setter+. */
export async function createRoute(
  locationId: string,
  body: RouteWriteShape,
  signal?: AbortSignal,
): Promise<RouteShape> {
  return request(`/locations/${locationId}/routes`, { method: 'POST', body, signal });
}

/** PUT /locations/{locationId}/routes/{routeId} — setter+. */
export async function updateRoute(
  locationId: string,
  routeId: string,
  body: RouteWriteShape,
  signal?: AbortSignal,
): Promise<RouteShape> {
  return request(`/locations/${locationId}/routes/${routeId}`, { method: 'PUT', body, signal });
}

/** PATCH /locations/{locationId}/routes/{routeId}/status — setter+. */
export async function updateRouteStatus(
  locationId: string,
  routeId: string,
  status: RouteStatus,
  signal?: AbortSignal,
): Promise<RouteShape> {
  return request(`/locations/${locationId}/routes/${routeId}/status`, {
    method: 'PATCH',
    body: { status },
    signal,
  });
}

/** DELETE /locations/{locationId}/routes/{routeId} — head_setter+ at the location. */
export async function deleteRoute(
  locationId: string,
  routeId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/routes/${routeId}`, { method: 'DELETE', signal });
}

// Ascents + ratings — surfaced on the route detail page. Hand-written
// shapes; mirror internal/model/Ascent + RouteRating.

export interface AscentShape {
  id: string;
  user_id: string;
  route_id: string;
  ascent_type: string;
  attempts: number;
  notes?: string | null;
  climbed_at: string;
  created_at: string;
}

export interface RouteRatingShape {
  id: string;
  user_id: string;
  route_id: string;
  rating: number;
  comment?: string | null;
  created_at: string;
  updated_at: string;
}

/** GET /locations/{locationId}/routes/{routeId}/ascents — paginated, descending. */
export async function listRouteAscents(
  locationId: string,
  routeId: string,
  limit = 20,
  signal?: AbortSignal,
): Promise<AscentShape[]> {
  const qs = limit ? `?limit=${limit}` : '';
  return request(`/locations/${locationId}/routes/${routeId}/ascents${qs}`, { signal });
}

/** GET /locations/{locationId}/routes/{routeId}/ratings — paginated, descending. */
export async function listRouteRatings(
  locationId: string,
  routeId: string,
  signal?: AbortSignal,
): Promise<RouteRatingShape[]> {
  return request(`/locations/${locationId}/routes/${routeId}/ratings`, { signal });
}

// ── Route photos ─────────────────────────────────────────────

export interface RoutePhotoShape {
  id: string;
  photo_url: string;
  sort_order: number;
  caption?: string | null;
  uploaded_by?: string | null;
  uploader_name?: string;
  created_at: string;
}

/** GET /locations/{locationId}/routes/{routeId}/photos — any member. */
export async function listRoutePhotos(
  locationId: string,
  routeId: string,
  signal?: AbortSignal,
): Promise<RoutePhotoShape[]> {
  return request(`/locations/${locationId}/routes/${routeId}/photos`, { signal });
}

/**
 * POST /locations/{locationId}/routes/{routeId}/photos — multipart upload.
 *
 * The server validates content type via byte-sniffing (allow-list:
 * jpeg/png/webp), caps the size at 5 MB and the per-route count at 20,
 * processes the image (resize/compress), and uploads to S3-compatible
 * storage. First photo on a route auto-becomes the primary.
 *
 * Bypasses the JSON-only `request()` helper since fetch needs the
 * browser to set the multipart boundary header itself.
 */
export async function uploadRoutePhoto(
  locationId: string,
  routeId: string,
  file: File,
  signal?: AbortSignal,
): Promise<RoutePhotoShape> {
  const fd = new FormData();
  fd.append('photo', file);
  const res = await fetch(`${API_BASE}/locations/${locationId}/routes/${routeId}/photos`, {
    method: 'POST',
    body: fd,
    credentials: 'same-origin',
    signal,
  });
  const text = await res.text();
  let payload: unknown;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = text;
    }
  }
  if (!res.ok) {
    throw new ApiClientError(res.status, payload as ApiError | string);
  }
  return payload as RoutePhotoShape;
}

/** DELETE /locations/{locationId}/routes/{routeId}/photos/{photoId} — uploader or setter+. */
export async function deleteRoutePhoto(
  locationId: string,
  routeId: string,
  photoId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(
    `/locations/${locationId}/routes/${routeId}/photos/${photoId}`,
    { method: 'DELETE', signal },
  );
}

// ── Community tags ──────────────────────────────────────────

export interface CommunityTagShape {
  tag_name: string;
  count: number;
  /** True when the current viewer added this tag. Drives the chip's "voted" state. */
  user_added: boolean;
}

/** GET /locations/{locationId}/routes/{routeId}/tags — any member. */
export async function listCommunityTags(
  locationId: string,
  routeId: string,
  signal?: AbortSignal,
): Promise<CommunityTagShape[]> {
  return request(`/locations/${locationId}/routes/${routeId}/tags`, { signal });
}

/**
 * POST /locations/{locationId}/routes/{routeId}/tags — any member adds a
 * tag. Server normalizes (trim/lowercase/collapse whitespace), enforces
 * 1–30 runes, runs the profanity filter; duplicates are a no-op.
 * Returns the updated aggregated list so the SPA can swap state in one
 * round-trip.
 */
export async function addCommunityTag(
  locationId: string,
  routeId: string,
  tagName: string,
  signal?: AbortSignal,
): Promise<CommunityTagShape[]> {
  return request(`/locations/${locationId}/routes/${routeId}/tags`, {
    method: 'POST',
    body: { tag_name: tagName },
    signal,
  });
}

/** DELETE /locations/{locationId}/routes/{routeId}/tags — drops the caller's vote. */
export async function removeCommunityTag(
  locationId: string,
  routeId: string,
  tagName: string,
  signal?: AbortSignal,
): Promise<CommunityTagShape[]> {
  return request(`/locations/${locationId}/routes/${routeId}/tags`, {
    method: 'DELETE',
    body: { tag_name: tagName },
    signal,
  });
}

/**
 * DELETE /locations/{locationId}/routes/{routeId}/tags/all — head_setter+
 * scrubs every vote for a tag from a route. Used by moderators to remove
 * misleading or off-topic tags entirely.
 */
export async function moderateCommunityTag(
  locationId: string,
  routeId: string,
  tagName: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/routes/${routeId}/tags/all`, {
    method: 'DELETE',
    body: { tag_name: tagName },
    signal,
  });
}

// ── Difficulty consensus ────────────────────────────────────

export type DifficultyVote = 'easy' | 'right' | 'hard';

export interface DifficultyConsensusShape {
  easy_count: number;
  right_count: number;
  hard_count: number;
  total_votes: number;
  /** Whole-percent breakdown that sums to ~100 (rounding may drop a point). */
  easy_pct: number;
  right_pct: number;
  hard_pct: number;
  /** Caller's prior vote, or "" if they haven't voted. */
  my_vote: '' | DifficultyVote;
}

/** GET /locations/{locationId}/routes/{routeId}/difficulty — any member. */
export async function getRouteDifficulty(
  locationId: string,
  routeId: string,
  signal?: AbortSignal,
): Promise<DifficultyConsensusShape> {
  return request(`/locations/${locationId}/routes/${routeId}/difficulty`, { signal });
}

/**
 * POST /locations/{locationId}/routes/{routeId}/difficulty — vote
 * easy/right/hard. One row per (user, route); resubmits upsert. Returns
 * the updated consensus + the caller's new vote.
 */
export async function voteRouteDifficulty(
  locationId: string,
  routeId: string,
  vote: DifficultyVote,
  signal?: AbortSignal,
): Promise<DifficultyConsensusShape> {
  return request(`/locations/${locationId}/routes/${routeId}/difficulty`, {
    method: 'POST',
    body: { vote },
    signal,
  });
}

export type AscentType = 'send' | 'flash' | 'attempt' | 'project';

export interface LogAscentShape {
  ascent_type: AscentType;
  attempts: number;
  notes?: string | null;
  /** ISO date or datetime; server defaults to now if omitted. */
  climbed_at?: string | null;
}

/** POST /locations/{locationId}/routes/{routeId}/ascent — any member. */
export async function logAscent(
  locationId: string,
  routeId: string,
  body: LogAscentShape,
  signal?: AbortSignal,
): Promise<AscentShape> {
  return request(`/locations/${locationId}/routes/${routeId}/ascent`, {
    method: 'POST',
    body,
    signal,
  });
}

export interface RateRouteShape {
  rating: number;
  comment?: string | null;
}

/**
 * POST /locations/{locationId}/routes/{routeId}/rate — any member. Server
 * upserts on (user, route) so re-rating overwrites the prior rating.
 */
export async function rateRoute(
  locationId: string,
  routeId: string,
  body: RateRouteShape,
  signal?: AbortSignal,
): Promise<RouteRatingShape> {
  return request(`/locations/${locationId}/routes/${routeId}/rate`, {
    method: 'POST',
    body,
    signal,
  });
}

// ── Setting sessions (Phase 2.4) ──────────────────────────
//
// Hand-written shapes — sessions aren't in the OpenAPI spec yet.
// Mirror `internal/model/SettingSession` + handler/session.go.

export type SessionStatus = 'planning' | 'in_progress' | 'complete' | 'cancelled';

export interface SessionAssignmentShape {
  id: string;
  session_id: string;
  setter_id: string;
  wall_id?: string | null;
  target_grades?: string[];
  notes?: string | null;
}

export interface SessionShape {
  id: string;
  location_id: string;
  scheduled_date: string;
  status: SessionStatus;
  notes?: string | null;
  created_by: string;
  assignments?: SessionAssignmentShape[];
  created_at: string;
  updated_at: string;
}

export interface SessionWriteShape {
  /** ISO date (YYYY-MM-DD). */
  scheduled_date: string;
  notes?: string | null;
}

/** GET /locations/{locationId}/sessions — setter+. */
export async function listSessions(
  locationId: string,
  signal?: AbortSignal,
): Promise<SessionShape[]> {
  return request(`/locations/${locationId}/sessions`, { signal });
}

/** GET /locations/{locationId}/sessions/{sessionId} — setter+. */
export async function getSession(
  locationId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<SessionShape> {
  return request(`/locations/${locationId}/sessions/${sessionId}`, { signal });
}

/** POST /locations/{locationId}/sessions — head_setter+. */
export async function createSession(
  locationId: string,
  body: SessionWriteShape,
  signal?: AbortSignal,
): Promise<SessionShape> {
  return request(`/locations/${locationId}/sessions`, { method: 'POST', body, signal });
}

/** PUT /locations/{locationId}/sessions/{sessionId} — head_setter+. */
export async function updateSession(
  locationId: string,
  sessionId: string,
  body: SessionWriteShape,
  signal?: AbortSignal,
): Promise<SessionShape> {
  return request(`/locations/${locationId}/sessions/${sessionId}`, {
    method: 'PUT',
    body,
    signal,
  });
}

/** DELETE /locations/{locationId}/sessions/{sessionId} — head_setter+. Soft delete. */
export async function deleteSession(
  locationId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/sessions/${sessionId}`, {
    method: 'DELETE',
    signal,
  });
}

/**
 * POST /locations/{locationId}/sessions/{sessionId}/status — head_setter+.
 * Simple status flip. Note: transitioning to 'complete' via this endpoint
 * does NOT publish draft routes or run the strip-targets pipeline; for
 * that, use the existing HTMX /sessions/{id}/complete view.
 */
export async function updateSessionStatus(
  locationId: string,
  sessionId: string,
  status: SessionStatus,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/sessions/${sessionId}/status`, {
    method: 'POST',
    body: { status },
    signal,
  });
}

export interface SessionPublishResultShape {
  stripped_route_count: number;
  published_routes: number;
  status: SessionStatus;
}

/**
 * POST /locations/{locationId}/sessions/{sessionId}/publish — head_setter+.
 *
 * One-shot completion: archives strip-targets, activates drafts, flips
 * status to complete. Returns counts so the SPA can show a confirmation
 * summary without re-fetching. Mirrors HTMX `/sessions/{id}/publish`.
 */
export async function publishSession(
  locationId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<SessionPublishResultShape> {
  return request(`/locations/${locationId}/sessions/${sessionId}/publish`, {
    method: 'POST',
    signal,
  });
}

/** POST /locations/{locationId}/sessions/{sessionId}/assignments — head_setter+. */
export async function addSessionAssignment(
  locationId: string,
  sessionId: string,
  body: {
    setter_id: string;
    wall_id?: string | null;
    target_grades?: string[];
    notes?: string | null;
  },
  signal?: AbortSignal,
): Promise<SessionAssignmentShape> {
  return request(`/locations/${locationId}/sessions/${sessionId}/assignments`, {
    method: 'POST',
    body,
    signal,
  });
}

/** DELETE /locations/{locationId}/sessions/{sessionId}/assignments/{assignmentId} — head_setter+. */
export async function removeSessionAssignment(
  locationId: string,
  sessionId: string,
  assignmentId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(
    `/locations/${locationId}/sessions/${sessionId}/assignments/${assignmentId}`,
    { method: 'DELETE', signal },
  );
}

export interface StripTargetShape {
  id: string;
  session_id: string;
  wall_id: string;
  route_id?: string | null;
  created_at: string;
  wall_name: string;
  wall_type: string;
  route_grade?: string | null;
  route_color?: string | null;
  route_name?: string | null;
  route_type?: string | null;
}

// ── Playbook (setter checklist template) ──────────────────────

export interface PlaybookStepShape {
  id: string;
  location_id: string;
  sort_order: number;
  title: string;
  created_at: string;
}

/** GET /locations/{locationId}/playbook — setter+ read. */
export async function listPlaybookSteps(
  locationId: string,
  signal?: AbortSignal,
): Promise<PlaybookStepShape[]> {
  return request(`/locations/${locationId}/playbook`, { signal });
}

/** POST /locations/{locationId}/playbook — head_setter+ append. */
export async function createPlaybookStep(
  locationId: string,
  title: string,
  signal?: AbortSignal,
): Promise<PlaybookStepShape> {
  return request(`/locations/${locationId}/playbook`, {
    method: 'POST',
    body: { title },
    signal,
  });
}

/** PATCH /locations/{locationId}/playbook/{stepId} — head_setter+ rename. */
export async function updatePlaybookStep(
  locationId: string,
  stepId: string,
  title: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/playbook/${stepId}`, {
    method: 'PATCH',
    body: { title },
    signal,
  });
}

/** DELETE /locations/{locationId}/playbook/{stepId} — head_setter+. */
export async function deletePlaybookStep(
  locationId: string,
  stepId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/playbook/${stepId}`, {
    method: 'DELETE',
    signal,
  });
}

/**
 * POST /locations/{locationId}/playbook/{stepId}/move — head_setter+.
 * Swaps with neighbor "up" or "down" in sort_order.
 */
export async function movePlaybookStep(
  locationId: string,
  stepId: string,
  direction: 'up' | 'down',
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/playbook/${stepId}/move`, {
    method: 'POST',
    body: { direction },
    signal,
  });
}

/** GET /locations/{locationId}/sessions/{sessionId}/strip-targets — setter+. */
export async function listStripTargets(
  locationId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<StripTargetShape[]> {
  return request(
    `/locations/${locationId}/sessions/${sessionId}/strip-targets`,
    { signal },
  );
}

/**
 * Routes linked to a setting session, enriched with setter + wall
 * names. Same shape the HTMX session-photos page uses.
 */
export interface SessionRouteDetailShape extends RouteShape {
  setter_name: string;
  wall_name: string;
}

/** GET /locations/{locationId}/sessions/{sessionId}/routes — setter+. */
export async function listSessionRoutes(
  locationId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<SessionRouteDetailShape[]> {
  return request(`/locations/${locationId}/sessions/${sessionId}/routes`, { signal });
}

/** POST /locations/{locationId}/sessions/{sessionId}/strip-targets — head_setter+. */
export async function addStripTarget(
  locationId: string,
  sessionId: string,
  body: { wall_id: string; route_id?: string | null },
  signal?: AbortSignal,
): Promise<StripTargetShape> {
  return request(`/locations/${locationId}/sessions/${sessionId}/strip-targets`, {
    method: 'POST',
    body,
    signal,
  });
}

/** DELETE /locations/{locationId}/sessions/{sessionId}/strip-targets/{targetId} — head_setter+. */
export async function removeStripTarget(
  locationId: string,
  sessionId: string,
  targetId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(
    `/locations/${locationId}/sessions/${sessionId}/strip-targets/${targetId}`,
    { method: 'DELETE', signal },
  );
}

export interface ChecklistItemShape {
  id: string;
  session_id: string;
  sort_order: number;
  title: string;
  completed: boolean;
  completed_by?: string | null;
  completed_at?: string | null;
  created_at: string;
  completed_by_name?: string | null;
}

/** GET /locations/{locationId}/sessions/{sessionId}/checklist — setter+. */
export async function listSessionChecklist(
  locationId: string,
  sessionId: string,
  signal?: AbortSignal,
): Promise<ChecklistItemShape[]> {
  return request(`/locations/${locationId}/sessions/${sessionId}/checklist`, {
    signal,
  });
}

/**
 * POST /locations/{locationId}/sessions/{sessionId}/checklist/{itemId}/toggle —
 * setter+. Returns the new completion count for the session.
 */
export async function toggleChecklistItem(
  locationId: string,
  sessionId: string,
  itemId: string,
  signal?: AbortSignal,
): Promise<{ completion_count: number }> {
  return request(
    `/locations/${locationId}/sessions/${sessionId}/checklist/${itemId}/toggle`,
    { method: 'POST', signal },
  );
}

// ── Card batches (Phase 2.5) ──────────────────────────────
//
// Hand-written shapes — card batches aren't in the OpenAPI spec yet.
// Mirror `internal/handler/card_batch.go::batchResponse` and the
// `internal/model/card_batch.go` constants.

export type CardBatchStatus = 'pending' | 'ready' | 'failed';
export type CardTheme = 'block_and_info' | 'full_color' | 'minimal' | 'trading_card';
export type CutterProfile = 'silhouette_type2';

export interface CardBatchShape {
  id: string;
  location_id: string;
  created_by: string;
  route_ids: string[];
  theme: CardTheme;
  cutter_profile: CutterProfile;
  status: CardBatchStatus;
  storage_key?: string | null;
  error_message?: string | null;
  page_count: number;
  created_at: string;
  updated_at: string;
}

export interface CardBatchCreateShape {
  route_ids: string[];
  theme?: CardTheme;
  cutter_profile?: CutterProfile;
}

export const CARD_THEMES: { value: CardTheme; label: string }[] = [
  { value: 'trading_card', label: 'Trading card' },
  { value: 'block_and_info', label: 'Block + info' },
  { value: 'full_color', label: 'Full color' },
  { value: 'minimal', label: 'Minimal' },
];

export const CUTTER_PROFILES: { value: CutterProfile; label: string }[] = [
  { value: 'silhouette_type2', label: 'Silhouette Cameo (Type 2)' },
];

/** GET /locations/{locationId}/card-batches — setter+. */
export async function listCardBatches(
  locationId: string,
  signal?: AbortSignal,
): Promise<CardBatchShape[]> {
  const res = await request<{ batches: CardBatchShape[] }>(
    `/locations/${locationId}/card-batches`,
    { signal },
  );
  return res.batches ?? [];
}

/** GET /locations/{locationId}/card-batches/{batchId} — setter+. */
export async function getCardBatch(
  locationId: string,
  batchId: string,
  signal?: AbortSignal,
): Promise<CardBatchShape> {
  return request(`/locations/${locationId}/card-batches/${batchId}`, { signal });
}

/** POST /locations/{locationId}/card-batches — setter+. */
export async function createCardBatch(
  locationId: string,
  body: CardBatchCreateShape,
  signal?: AbortSignal,
): Promise<CardBatchShape> {
  return request(`/locations/${locationId}/card-batches`, {
    method: 'POST',
    body,
    signal,
  });
}

/** DELETE /locations/{locationId}/card-batches/{batchId} — setter+. */
export async function deleteCardBatch(
  locationId: string,
  batchId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/card-batches/${batchId}`, {
    method: 'DELETE',
    signal,
  });
}

/**
 * PATCH /locations/{locationId}/card-batches/{batchId} — creator or
 * head_setter+. Replaces the batch's route_ids; the next download
 * re-renders against the new selection.
 */
export async function updateCardBatch(
  locationId: string,
  batchId: string,
  routeIds: string[],
  signal?: AbortSignal,
): Promise<CardBatchShape> {
  return request(`/locations/${locationId}/card-batches/${batchId}`, {
    method: 'PATCH',
    body: { route_ids: routeIds },
    signal,
  });
}

/** Resolve the absolute download URL for a card batch's PDF. */
export function cardBatchDownloadUrl(locationId: string, batchId: string): string {
  return `/api/v1/locations/${locationId}/card-batches/${batchId}/pdf`;
}

/**
 * Resolve the cutlines DXF URL for a card batch. The DXF is a fallback
 * for Silhouette Studio when Cut-by-Color misbehaves on the PDF.
 *
 * Served by the HTMX-side handler at /card-batches/{id}/cutlines.dxf —
 * kept after the SPA-to-root swap because the SPA links to it as a
 * download target. Uses the cookie session for auth.
 */
export function cardBatchCutlinesUrl(batchId: string): string {
  return `/card-batches/${batchId}/cutlines.dxf`;
}

/**
 * Resolve the first-card preview PNG URL for a card batch. Used as an
 * <img src> on the batch detail page so setters get a quick visual
 * confirmation that their print settings look right.
 *
 * Same provenance as cardBatchCutlinesUrl — server-rendered artifact
 * the SPA references, kept after the swap.
 */
export function cardBatchPreviewUrl(batchId: string): string {
  return `/card-batches/${batchId}/preview.png`;
}

/**
 * POST /locations/{locationId}/card-batches/{batchId}/retry — creator
 * or head_setter+. Resets a failed batch back to pending so the next
 * download re-renders. Returns the updated batch.
 */
export async function retryCardBatch(
  locationId: string,
  batchId: string,
  signal?: AbortSignal,
): Promise<CardBatchShape> {
  return request(`/locations/${locationId}/card-batches/${batchId}/retry`, {
    method: 'POST',
    signal,
  });
}

/**
 * GET /api/v1/me — returns the authenticated user + their memberships.
 * Resolves with `null` on 401 (caller should redirect to login).
 */
export async function getMe(signal?: AbortSignal): Promise<MeResponse | null> {
  try {
    return await request<MeResponse>('/me', { signal });
  } catch (err) {
    if (err instanceof ApiClientError && err.status === 401) {
      return null;
    }
    throw err;
  }
}

/** GET /locations/{locationId}/competitions */
export async function listCompetitions(
  locationId: string,
  status?: CompetitionStatus,
  signal?: AbortSignal,
): Promise<Competition[]> {
  const q = status ? `?status=${encodeURIComponent(status)}` : '';
  return request(`/locations/${locationId}/competitions${q}`, { signal });
}

// ── /me writes + climber stats (Phase 2.6) ─────────────────

export interface UpdateMeShape {
  display_name?: string;
  avatar_url?: string | null;
  bio?: string | null;
  /** Set true (with omitted/null avatar_url) to clear the existing avatar. */
  clear_avatar_url?: boolean;
  /** Set true (with omitted/null bio) to clear the existing bio. */
  clear_bio?: boolean;
}

export interface UpdateMeResponse {
  user: UserShape;
}

/**
 * PATCH /api/v1/me — patch caller's profile fields. Pass `clear_*: true`
 * along with omitted/null on the field to explicitly clear it (since the
 * server can't distinguish "missing" from "JSON null" otherwise).
 */
export async function updateMe(
  body: UpdateMeShape,
  signal?: AbortSignal,
): Promise<UserShape> {
  const res = await request<UpdateMeResponse>('/me', { method: 'PATCH', body, signal });
  return res.user;
}

/** POST /api/v1/me/password — change password. Returns 204. */
export async function changePassword(
  oldPassword: string,
  newPassword: string,
  signal?: AbortSignal,
): Promise<void> {
  return request('/me/password', {
    method: 'POST',
    body: { old_password: oldPassword, new_password: newPassword },
    signal,
  });
}

/**
 * PUT /api/v1/me/view-as — set or clear the view-as role override.
 * Pass `null` (or empty string) to clear. Server enforces: caller must be
 * head_setter+ AND target rank < caller's. Returns 204; the SPA then
 * reloads /me to pick up the new effective role.
 */
export async function setViewAs(
  role: string | null,
  signal?: AbortSignal,
): Promise<void> {
  return request('/me/view-as', {
    method: 'PUT',
    body: { role: role ?? '' },
    signal,
  });
}

export interface AscentWithRouteShape {
  id: string;
  user_id: string;
  route_id: string;
  ascent_type: string;
  attempts: number;
  notes?: string | null;
  climbed_at: string;
  created_at: string;
  route_grade: string;
  route_grading_system: string;
  route_type: string;
  route_color: string;
  route_name?: string | null;
  wall_id: string;
}

export interface MyAscentsResponse {
  ascents: AscentWithRouteShape[];
  total: number;
}

/** GET /api/v1/me/ascents — paginated list of caller's ticks (newest first). */
export async function listMyAscents(
  limit = 25,
  offset = 0,
  signal?: AbortSignal,
): Promise<MyAscentsResponse> {
  const qs = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return request(`/me/ascents?${qs}`, { signal });
}

export interface UpdateAscentShape {
  ascent_type: AscentType;
  attempts: number;
  notes?: string | null;
}

/**
 * PATCH /api/v1/me/ascents/{ascentId} — owner-only edit. Type, attempt
 * count, and notes are mutable; route + climbed_at are not (start a
 * fresh log entry for a different climb).
 */
export async function updateMyAscent(
  ascentId: string,
  body: UpdateAscentShape,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/me/ascents/${ascentId}`, { method: 'PATCH', body, signal });
}

/** DELETE /api/v1/me/ascents/{ascentId} — owner-only delete. */
export async function deleteMyAscent(
  ascentId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/me/ascents/${ascentId}`, { method: 'DELETE', signal });
}

export interface GradePyramidEntryShape {
  grading_system: string;
  grade: string;
  count: number;
}

export interface MyStatsShape {
  total_sends: number;
  total_flashes: number;
  total_logged: number;
  unique_routes: number;
}

/**
 * GET /api/v1/me/stats — climber summary counters.
 *
 * Returns the four denormalized totals (sends/flashes/logged/unique
 * routes) read directly from the users row. Sub-millisecond regardless
 * of how many ascents the climber has logged. The grade pyramid is its
 * own endpoint (getMyGradePyramid below); the profile page lazy-loads
 * it so the cheap summary panel renders immediately.
 */
export async function getMyStats(signal?: AbortSignal): Promise<MyStatsShape> {
  return request('/me/stats', { signal });
}

/**
 * GET /api/v1/me/grade-pyramid — sends grouped by grade.
 *
 * Split from /me/stats so the cheap summary doesn't pay for the GROUP
 * BY scan on every profile load. Empty array when the climber has no
 * sends.
 */
export async function getMyGradePyramid(signal?: AbortSignal): Promise<GradePyramidEntryShape[]> {
  return request('/me/grade-pyramid', { signal });
}

// ── Notifications ──────────────────────────────────────────

export interface NotificationShape {
  id: number;
  user_id: string;
  type: string;
  title: string;
  body: string;
  link?: string | null;
  read_at?: string | null;
  created_at: string;
}

/** GET /api/v1/me/notifications — caller's unread notifications, newest first. */
export async function listMyNotifications(
  limit = 50,
  signal?: AbortSignal,
): Promise<NotificationShape[]> {
  const res = await request<{ notifications: NotificationShape[] }>(
    `/me/notifications?limit=${limit}`,
    { signal },
  );
  return res.notifications ?? [];
}

/** GET /api/v1/me/notifications/unread-count — cheap badge poll. */
export async function getUnreadNotificationCount(signal?: AbortSignal): Promise<number> {
  const res = await request<{ count: number }>('/me/notifications/unread-count', { signal });
  return res.count ?? 0;
}

/** POST /api/v1/me/notifications/{id}/read */
export async function markNotificationRead(
  id: number,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/me/notifications/${id}/read`, { method: 'POST', signal });
}

/** POST /api/v1/me/notifications/read-all */
export async function markAllNotificationsRead(
  signal?: AbortSignal,
): Promise<{ marked: number }> {
  return request('/me/notifications/read-all', { method: 'POST', signal });
}

// ── Setter dashboard ───────────────────────────────────────

export interface DashboardStatsShape {
  active_routes: number;
  active_delta: number;
  total_sends_30d: number;
  avg_rating: number;
  due_for_strip: number;
  set_this_week: number;
  set_last_week: number;
}

export interface DashboardActivityShape {
  user_name: string;
  ascent_type: string;
  time: string;
  route_color: string;
  route_grade: string;
  route_grading_system: string;
  route_circuit_color?: string | null;
  route_name?: string | null;
}

export interface DashboardSummaryShape {
  stats: DashboardStatsShape;
  recent_activity: DashboardActivityShape[];
}

/**
 * GET /api/v1/locations/{locationId}/dashboard — setter+. Combined stats
 * + recent activity for the SPA setter dashboard at /app. The HTMX
 * dashboard composes the same numbers inline; this endpoint surfaces
 * them as JSON.
 */
export async function getDashboardSummary(
  locationId: string,
  signal?: AbortSignal,
): Promise<DashboardSummaryShape> {
  return request(`/locations/${locationId}/dashboard`, { signal });
}

// ── Team management (Phase 2.7) ────────────────────────────

export type MembershipRole =
  | 'climber'
  | 'setter'
  | 'head_setter'
  | 'gym_manager'
  | 'org_admin';

export interface TeamMemberShape {
  membership_id: string;
  user_id: string;
  display_name: string;
  email: string;
  role: MembershipRole;
}

export interface TeamListResponse {
  members: TeamMemberShape[];
  total_count: number;
}

/** GET /api/v1/locations/{locationId}/team — head_setter+. */
export async function listTeam(
  locationId: string,
  filters: { q?: string; role?: MembershipRole } = {},
  signal?: AbortSignal,
): Promise<TeamListResponse> {
  const qs = new URLSearchParams();
  if (filters.q) qs.set('q', filters.q);
  if (filters.role) qs.set('role', filters.role);
  const suffix = qs.toString() ? `?${qs}` : '';
  return request(`/locations/${locationId}/team${suffix}`, { signal });
}

/**
 * GET /api/v1/orgs/{orgId}/team — org_admin only. Org-wide member list,
 * same response shape as listTeam so the SPA can swap source URL.
 */
export async function listOrgTeam(
  orgId: string,
  filters: { q?: string; role?: MembershipRole } = {},
  signal?: AbortSignal,
): Promise<TeamListResponse> {
  const qs = new URLSearchParams();
  if (filters.q) qs.set('q', filters.q);
  if (filters.role) qs.set('role', filters.role);
  const suffix = qs.toString() ? `?${qs}` : '';
  return request(`/orgs/${orgId}/team${suffix}`, { signal });
}

/** PATCH /api/v1/memberships/{membershipId} — gym_manager+ to assign elevated roles. */
export async function updateMembership(
  membershipId: string,
  role: MembershipRole,
  signal?: AbortSignal,
): Promise<void> {
  await request(`/memberships/${membershipId}`, {
    method: 'PATCH',
    body: { role },
    signal,
  });
}

/** DELETE /api/v1/memberships/{membershipId} — gym_manager+. Soft-deletes. */
export async function removeMembership(
  membershipId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/memberships/${membershipId}`, { method: 'DELETE', signal });
}

// ── Quests (Phase 2.8) ─────────────────────────────────────
//
// Hand-written shapes — quests aren't in the OpenAPI spec.
// Mirror internal/model/quest.go + internal/repository/quest.go::QuestListItem.

export type ClimberQuestStatus = 'active' | 'completed' | 'abandoned';

export interface QuestDomainShape {
  id: string;
  name: string;
  color?: string | null;
  icon?: string | null;
}

export interface QuestShape {
  id: string;
  location_id: string;
  domain_id: string;
  badge_id?: string | null;
  name: string;
  description: string;
  quest_type: string;
  completion_criteria: string;
  target_count?: number | null;
  suggested_duration_days?: number | null;
  available_from?: string | null;
  available_until?: string | null;
  skill_level: string;
  requires_certification?: string | null;
  route_tag_filter?: string[];
  is_active: boolean;
  sort_order: number;
  created_at: string;
  updated_at: string;
  domain?: QuestDomainShape | null;
}

export interface QuestListItemShape extends QuestShape {
  active_count: number;
  completed_count: number;
}

export interface ClimberQuestShape {
  id: string;
  user_id: string;
  quest_id: string;
  status: ClimberQuestStatus;
  progress_count: number;
  started_at: string;
  completed_at?: string | null;
  quest?: QuestShape | null;
}

export interface QuestLogShape {
  id: string;
  climber_quest_id: string;
  log_type: string;
  route_id?: string | null;
  notes?: string | null;
  rating?: number | null;
  logged_at: string;
}

/** GET /api/v1/locations/{locationId}/quests — active catalog at this location. */
export async function listQuests(
  locationId: string,
  signal?: AbortSignal,
): Promise<QuestListItemShape[]> {
  const res = await request<{ quests: QuestListItemShape[] }>(
    `/locations/${locationId}/quests`,
    { signal },
  );
  return res.quests ?? [];
}

/** GET /api/v1/me/quests — caller's enrollments, optional status filter. */
export async function listMyQuests(
  status?: ClimberQuestStatus,
  signal?: AbortSignal,
): Promise<ClimberQuestShape[]> {
  const qs = status ? `?status=${status}` : '';
  const res = await request<{ quests: ClimberQuestShape[] }>(`/me/quests${qs}`, { signal });
  return res.quests ?? [];
}

/** GET /api/v1/quests/{questId} — quest detail (no enrollment context). */
export async function getQuest(questId: string, signal?: AbortSignal): Promise<QuestShape> {
  return request(`/quests/${questId}`, { signal });
}

export interface StartQuestResponse {
  enrollment?: ClimberQuestShape;
  already_enrolled?: boolean;
}

/** POST /api/v1/quests/{questId}/start — enroll caller. */
export async function startQuest(
  questId: string,
  locationId: string,
  signal?: AbortSignal,
): Promise<StartQuestResponse> {
  return request(`/quests/${questId}/start`, {
    method: 'POST',
    body: { location_id: locationId },
    signal,
  });
}

export interface LogQuestProgressShape {
  log_type?: string;
  notes?: string | null;
  route_id?: string | null;
  rating?: number | null;
}

/** POST /api/v1/climber-quests/{climberQuestId}/log */
export async function logQuestProgress(
  climberQuestId: string,
  body: LogQuestProgressShape,
  signal?: AbortSignal,
): Promise<QuestLogShape> {
  const res = await request<{ log: QuestLogShape }>(
    `/climber-quests/${climberQuestId}/log`,
    { method: 'POST', body, signal },
  );
  return res.log;
}

/** DELETE /api/v1/climber-quests/{climberQuestId} — abandon. */
export async function abandonQuest(
  climberQuestId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/climber-quests/${climberQuestId}`, { method: 'DELETE', signal });
}

// ── Progressions admin (head_setter+) ─────────────────────

/** GET /api/v1/locations/{id}/admin/quests — every quest, including inactive. */
export async function listAllQuests(
  locationId: string,
  signal?: AbortSignal,
): Promise<QuestListItemShape[]> {
  const res = await request<{ quests: QuestListItemShape[] }>(
    `/locations/${locationId}/admin/quests`,
    { signal },
  );
  return res.quests ?? [];
}

/** POST /api/v1/locations/{id}/admin/quests. Body matches the Quest model. */
export async function createQuest(
  locationId: string,
  body: Partial<QuestShape>,
  signal?: AbortSignal,
): Promise<QuestShape> {
  return request(`/locations/${locationId}/admin/quests`, {
    method: 'POST',
    body,
    signal,
  });
}

/** PUT /api/v1/quests/{questId}. */
export async function updateQuest(
  questId: string,
  body: Partial<QuestShape>,
  signal?: AbortSignal,
): Promise<QuestShape> {
  return request(`/quests/${questId}`, { method: 'PUT', body, signal });
}

/** POST /api/v1/locations/{id}/admin/quests/{questId}/deactivate. */
export async function deactivateQuest(
  locationId: string,
  questId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/admin/quests/${questId}/deactivate`, {
    method: 'POST',
    signal,
  });
}

/** POST /api/v1/locations/{id}/admin/quests/{questId}/duplicate. */
export async function duplicateQuest(
  locationId: string,
  questId: string,
  signal?: AbortSignal,
): Promise<QuestShape> {
  return request(`/locations/${locationId}/admin/quests/${questId}/duplicate`, {
    method: 'POST',
    signal,
  });
}

export interface QuestDomainShapeFull {
  id: string;
  location_id: string;
  name: string;
  description?: string | null;
  color?: string | null;
  icon?: string | null;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

/** GET /api/v1/locations/{id}/admin/quest-domains. */
export async function listQuestDomains(
  locationId: string,
  signal?: AbortSignal,
): Promise<QuestDomainShapeFull[]> {
  return request(`/locations/${locationId}/admin/quest-domains`, { signal });
}

/** POST /api/v1/locations/{id}/admin/quest-domains. */
export async function createQuestDomain(
  locationId: string,
  body: Partial<QuestDomainShapeFull>,
  signal?: AbortSignal,
): Promise<QuestDomainShapeFull> {
  return request(`/locations/${locationId}/admin/quest-domains`, {
    method: 'POST',
    body,
    signal,
  });
}

/** PUT /api/v1/quest-domains/{id}. */
export async function updateQuestDomain(
  domainId: string,
  body: Partial<QuestDomainShapeFull>,
  signal?: AbortSignal,
): Promise<QuestDomainShapeFull> {
  return request(`/quest-domains/${domainId}`, { method: 'PUT', body, signal });
}

/** DELETE /api/v1/locations/{id}/admin/quest-domains/{domainId}. */
export async function deleteQuestDomain(
  locationId: string,
  domainId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/locations/${locationId}/admin/quest-domains/${domainId}`, {
    method: 'DELETE',
    signal,
  });
}

export interface BadgeShape {
  id: string;
  location_id: string;
  name: string;
  description?: string | null;
  icon: string;
  color: string;
  created_at: string;
}

/** GET /api/v1/locations/{id}/admin/badges. */
export async function listBadges(
  locationId: string,
  signal?: AbortSignal,
): Promise<BadgeShape[]> {
  return request(`/locations/${locationId}/admin/badges`, { signal });
}

export interface BadgeShowcaseEntry {
  id: string;
  name: string;
  description?: string | null;
  icon: string;
  color: string;
  /** ISO timestamp when the caller earned the badge; absent when unearned. */
  earned_at?: string;
}

export interface ActivityEntryShape {
  id: string;
  location_id: string;
  user_id: string;
  activity_type: string;
  entity_type: string;
  entity_id: string;
  metadata: Record<string, unknown>;
  created_at: string;
  user_display_name?: string;
  user_avatar_url?: string | null;
}

/**
 * GET /api/v1/locations/{id}/activity?limit=50&offset=0&type=...
 * Location-wide activity feed (quest progress, badges, route sets).
 * Any location member can read.
 */
export async function listLocationActivity(
  locationId: string,
  opts: { limit?: number; offset?: number; type?: string } = {},
  signal?: AbortSignal,
): Promise<ActivityEntryShape[]> {
  const qs = new URLSearchParams();
  if (opts.limit != null) qs.set('limit', String(opts.limit));
  if (opts.offset != null) qs.set('offset', String(opts.offset));
  if (opts.type) qs.set('type', opts.type);
  const suffix = qs.toString() ? `?${qs}` : '';
  return request(`/locations/${locationId}/activity${suffix}`, { signal });
}

/**
 * GET /api/v1/locations/{id}/badges/showcase — climber-facing view.
 * Returns the location's catalog with `earned_at` populated for each
 * badge the caller has earned; unearned badges have no earned_at.
 */
export async function getBadgeShowcase(
  locationId: string,
  signal?: AbortSignal,
): Promise<BadgeShowcaseEntry[]> {
  return request(`/locations/${locationId}/badges/showcase`, { signal });
}

/** POST /api/v1/locations/{id}/admin/badges. */
export async function createBadge(
  locationId: string,
  body: Partial<BadgeShape>,
  signal?: AbortSignal,
): Promise<BadgeShape> {
  return request(`/locations/${locationId}/admin/badges`, {
    method: 'POST',
    body,
    signal,
  });
}

/** PUT /api/v1/badges/{id}. */
export async function updateBadge(
  badgeId: string,
  body: Partial<BadgeShape>,
  signal?: AbortSignal,
): Promise<BadgeShape> {
  return request(`/badges/${badgeId}`, { method: 'PUT', body, signal });
}

/** DELETE /api/v1/badges/{id}. */
export async function deleteBadge(
  badgeId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/badges/${badgeId}`, { method: 'DELETE', signal });
}

/** GET /competitions/{id} */
export async function getCompetition(id: string, signal?: AbortSignal): Promise<Competition> {
  return request(`/competitions/${id}`, { signal });
}

/** POST /locations/{locationId}/competitions */
export async function createCompetition(
  locationId: string,
  body: CompetitionCreate,
  signal?: AbortSignal,
): Promise<Competition> {
  return request(`/locations/${locationId}/competitions`, {
    method: 'POST',
    body,
    signal,
  });
}

/** PATCH /competitions/{id} */
export async function updateCompetition(
  id: string,
  body: CompetitionUpdate,
  signal?: AbortSignal,
): Promise<Competition> {
  return request(`/competitions/${id}`, { method: 'PATCH', body, signal });
}

/**
 * GET /competitions/by-slug/{slug} — resolve by slug across the user's
 * accessible locations. Used when a magic-link URL has a slug but no
 * location context.
 */
export async function getCompetitionBySlug(
  slug: string,
  signal?: AbortSignal,
): Promise<Competition> {
  return request(`/competitions/by-slug/${encodeURIComponent(slug)}`, { signal });
}

/** POST /competitions/{id}/events */
export async function createEvent(
  competitionId: string,
  body: EventCreate,
  signal?: AbortSignal,
): Promise<CompetitionEvent> {
  return request(`/competitions/${competitionId}/events`, {
    method: 'POST',
    body,
    signal,
  });
}

/** PATCH /events/{id} */
export async function updateEvent(
  eventId: string,
  body: EventUpdate,
  signal?: AbortSignal,
): Promise<CompetitionEvent> {
  return request(`/events/${eventId}`, { method: 'PATCH', body, signal });
}

/** POST /competitions/{id}/categories */
export async function createCategory(
  competitionId: string,
  body: CategoryCreate,
  signal?: AbortSignal,
): Promise<CompetitionCategory> {
  return request(`/competitions/${competitionId}/categories`, {
    method: 'POST',
    body,
    signal,
  });
}

/** GET /competitions/{id}/events */
export async function listEvents(
  competitionId: string,
  signal?: AbortSignal,
): Promise<CompetitionEvent[]> {
  return request(`/competitions/${competitionId}/events`, { signal });
}

/** GET /competitions/{id}/categories */
export async function listCategories(
  competitionId: string,
  signal?: AbortSignal,
): Promise<CompetitionCategory[]> {
  return request(`/competitions/${competitionId}/categories`, { signal });
}

/** GET /events/{id}/problems */
export async function listProblems(
  eventId: string,
  signal?: AbortSignal,
): Promise<CompetitionProblem[]> {
  return request(`/events/${eventId}/problems`, { signal });
}

/** POST /events/{id}/problems */
export async function createProblem(
  eventId: string,
  body: ProblemCreate,
  signal?: AbortSignal,
): Promise<CompetitionProblem> {
  return request(`/events/${eventId}/problems`, {
    method: 'POST',
    body,
    signal,
  });
}

/** PATCH /problems/{id} */
export async function updateProblem(
  problemId: string,
  body: ProblemUpdate,
  signal?: AbortSignal,
): Promise<CompetitionProblem> {
  return request(`/problems/${problemId}`, { method: 'PATCH', body, signal });
}

/** GET /competitions/{id}/registrations */
export async function listRegistrations(
  competitionId: string,
  signal?: AbortSignal,
): Promise<CompetitionRegistration[]> {
  return request(`/competitions/${competitionId}/registrations`, { signal });
}

/** POST /competitions/{id}/registrations */
export async function createRegistration(
  competitionId: string,
  body: RegistrationCreate,
  signal?: AbortSignal,
): Promise<CompetitionRegistration> {
  return request(`/competitions/${competitionId}/registrations`, {
    method: 'POST',
    body,
    signal,
  });
}

/** GET /registrations/{id}/attempts */
export async function listRegistrationAttempts(
  registrationId: string,
  signal?: AbortSignal,
): Promise<AttemptState[]> {
  return request(`/registrations/${registrationId}/attempts`, { signal });
}

/** DELETE /registrations/{id} — soft withdraw (frees the bib). */
export async function withdrawRegistration(
  registrationId: string,
  signal?: AbortSignal,
): Promise<void> {
  return request(`/registrations/${registrationId}`, { method: 'DELETE', signal });
}

/**
 * POST /competitions/{id}/actions — submit one or more attempt actions.
 * Used by the climber scorecard's action queue with idempotency keys
 * for retry-safe writes.
 */
export async function submitActions(
  competitionId: string,
  body: ActionsRequest,
  signal?: AbortSignal,
): Promise<ActionsResponse> {
  return request(`/competitions/${competitionId}/actions`, {
    method: 'POST',
    body,
    signal,
  });
}
