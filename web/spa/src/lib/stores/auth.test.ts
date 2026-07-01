// Tests for the auth store's loadMe() lifecycle: success, confirmed 401,
// and transient-failure retry semantics. The store keeps module-level
// $state, so each test re-imports a fresh copy via vi.resetModules().

import { afterEach, describe, expect, it, vi } from 'vitest';
import type { MeResponse } from '$lib/api/client';

vi.mock('$lib/api/client', () => ({
  getMe: vi.fn(),
}));

const ME: MeResponse = {
  user: {
    id: 'u1',
    email: 'x@example.com',
    display_name: 'X',
    is_app_admin: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  memberships: [],
};

/** Fresh store + mocked getMe per test — module state must not leak. */
async function freshStore() {
  vi.resetModules();
  const client = await import('$lib/api/client');
  const auth = await import('$lib/stores/auth.svelte');
  const getMe = vi.mocked(client.getMe);
  // The mock instance survives resetModules — drop calls + behaviors so
  // each test starts clean.
  getMe.mockReset();
  return { getMe, ...auth };
}

afterEach(() => {
  vi.useRealTimers();
});

describe('loadMe', () => {
  it('success sets me and clears unauthenticated', async () => {
    const { getMe, loadMe, authState } = await freshStore();
    getMe.mockResolvedValueOnce(ME);

    await loadMe();

    const s = authState();
    expect(s.me).toEqual(ME);
    expect(s.loaded).toBe(true);
    expect(s.loading).toBe(false);
    expect(s.unauthenticated).toBe(false);
    expect(s.error).toBeNull();
  });

  it('a confirmed 401 (getMe → null) sets unauthenticated without retrying', async () => {
    const { getMe, loadMe, authState } = await freshStore();
    getMe.mockResolvedValueOnce(null);

    await loadMe();

    const s = authState();
    expect(s.me).toBeNull();
    expect(s.loaded).toBe(true);
    expect(s.unauthenticated).toBe(true);
    expect(s.error).toBeNull();
    expect(getMe).toHaveBeenCalledTimes(1);
  });

  it('retries a transient failure with backoff and succeeds', async () => {
    vi.useFakeTimers();
    const { getMe, loadMe, authState } = await freshStore();
    getMe
      .mockRejectedValueOnce(new Error('network blip'))
      .mockRejectedValueOnce(new Error('network blip'))
      .mockResolvedValueOnce(ME);

    const settled = loadMe();
    await vi.runAllTimersAsync(); // flush the 400ms/800ms backoff sleeps
    await settled;

    const s = authState();
    expect(getMe).toHaveBeenCalledTimes(3);
    expect(s.me).toEqual(ME);
    expect(s.unauthenticated).toBe(false);
    expect(s.error).toBeNull();
  });

  it('exhausted retries preserve me and surface the error without unauthenticated', async () => {
    const { getMe, loadMe, authState } = await freshStore();
    // Seed a logged-in state first — the transient failure must not drop it.
    getMe.mockResolvedValueOnce(ME);
    await loadMe();

    vi.useFakeTimers();
    getMe.mockRejectedValue(new Error('server down'));

    const settled = loadMe();
    await vi.runAllTimersAsync();
    await settled;

    const s = authState();
    expect(getMe).toHaveBeenCalledTimes(1 + 4); // initial + 1 try + 3 retries
    expect(s.me).toEqual(ME); // session preserved
    expect(s.unauthenticated).toBe(false); // NOT mistaken for a logout
    expect(s.error).toBeInstanceOf(Error);
    expect(s.error?.message).toBe('server down');
    expect(s.loaded).toBe(true);
    expect(s.loading).toBe(false);
  });

  it('parallel calls dedupe onto one in-flight fetch', async () => {
    const { getMe, loadMe } = await freshStore();
    getMe.mockResolvedValue(ME);

    await Promise.all([loadMe(), loadMe()]);

    expect(getMe).toHaveBeenCalledTimes(1);
  });
});
