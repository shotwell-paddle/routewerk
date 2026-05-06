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

/** GET /locations/{locationId}/walls — non-archived walls, ascending sort. */
export async function listWalls(locationId: string, signal?: AbortSignal): Promise<WallShape[]> {
  return request(`/locations/${locationId}/walls`, { signal });
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

/** Resolve the absolute download URL for a card batch's PDF. */
export function cardBatchDownloadUrl(locationId: string, batchId: string): string {
  return `/api/v1/locations/${locationId}/card-batches/${batchId}/pdf`;
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
  grade_pyramid: GradePyramidEntryShape[];
}

/** GET /api/v1/me/stats — climber summary + grade pyramid. */
export async function getMyStats(signal?: AbortSignal): Promise<MyStatsShape> {
  return request('/me/stats', { signal });
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
