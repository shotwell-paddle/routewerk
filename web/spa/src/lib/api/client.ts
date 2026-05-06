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
export type Aggregation = Schemas['Aggregation'];
export type MagicLinkRequest = Schemas['MagicLinkRequest'];
export type ApiError = Schemas['Error'];

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
