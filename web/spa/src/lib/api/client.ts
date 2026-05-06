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
