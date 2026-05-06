// Auth store — single source of truth for "who is the current user?"
// across the SPA. Lazy: doesn't fetch /me until something asks.
//
// SvelteKit is configured for SPA mode (no SSR), so this runs in the
// browser only. The session cookie is sent automatically with every
// fetch (`credentials: 'same-origin'` in client.ts) — we don't store
// any token here, just the resolved user metadata.
//
// Rune-based store: components subscribe via `$state` reactivity.
// `loadMe()` on app boot or after a login event populates the cache;
// `clear()` on logout drops it.

import { getMe, type MeResponse } from '$lib/api/client';

interface AuthState {
  /** Last result from /me. null = not loaded OR unauthenticated. */
  me: MeResponse | null;
  /** True while a /me fetch is in flight (initial load or refresh). */
  loading: boolean;
  /** True once the first /me fetch has settled (success OR 401). */
  loaded: boolean;
  /** The error from the last /me fetch, if any. */
  error: Error | null;
}

const state = $state<AuthState>({
  me: null,
  loading: false,
  loaded: false,
  error: null,
});

/** The current auth state — components read fields from this. */
export function authState() {
  return state;
}

/** Convenience: the current user, or null if not authenticated. */
export function currentUser() {
  return state.me?.user ?? null;
}

/** True iff /me has loaded AND returned a user. */
export function isAuthenticated() {
  return state.loaded && state.me !== null;
}

/**
 * Fetch /me and update the store. Idempotent: parallel calls dedupe
 * via the loading flag. Resolves once the fetch settles regardless
 * of success/failure (caller can then read state).
 */
let inflight: Promise<void> | null = null;
export function loadMe(): Promise<void> {
  if (inflight) return inflight;
  state.loading = true;
  state.error = null;
  inflight = (async () => {
    try {
      state.me = await getMe();
    } catch (err) {
      state.error = err instanceof Error ? err : new Error(String(err));
      state.me = null;
    } finally {
      state.loading = false;
      state.loaded = true;
      inflight = null;
    }
  })();
  return inflight;
}

/** Drop cached user state — call after logout. */
export function clear() {
  state.me = null;
  state.loaded = true;
  state.error = null;
}
