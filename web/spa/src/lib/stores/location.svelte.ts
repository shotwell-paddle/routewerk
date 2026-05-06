// Location store — tracks the user's currently-selected location across
// the SPA. Backed by localStorage so the choice survives reloads.
//
// The user can be a member of multiple locations (multi-gym staff,
// climbers who joined several gyms). The sidebar's location picker
// writes here; pages that scope by location read from here.

import { authState } from './auth.svelte';

const STORAGE_KEY = 'rw.selectedLocationId';

interface LocationState {
  /** UUID of the location the user is acting in. null until first set. */
  selectedId: string | null;
}

const state = $state<LocationState>({
  selectedId: typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEY) : null,
});

export function locationState() {
  return state;
}

export function setSelectedLocation(id: string | null) {
  state.selectedId = id;
  if (typeof localStorage !== 'undefined') {
    if (id) localStorage.setItem(STORAGE_KEY, id);
    else localStorage.removeItem(STORAGE_KEY);
  }
}

/**
 * Resolve the effective selected location. Falls back to the first
 * non-null `location_id` membership if nothing is stored — gives the
 * sidebar a sensible default for users with a single gym.
 */
export function effectiveLocationId(): string | null {
  if (state.selectedId) return state.selectedId;
  const me = authState().me;
  if (!me) return null;
  const first = me.memberships.find((m) => m.location_id);
  return first?.location_id ?? null;
}
