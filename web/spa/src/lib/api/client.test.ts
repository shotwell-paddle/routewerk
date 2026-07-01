// Tests for the fetch wrapper (`request`) via its exported endpoint
// functions, plus getMe's 401-vs-error contract. `request` itself is
// module-private, so we exercise it through thin wrappers:
//   deleteSession  → DELETE, 204 handling
//   listWalls      → GET, JSON body parsing
//   requestMagicLink → POST, JSON request body
//   getMe          → 401 → null, everything else throws

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ApiClientError,
  deleteSession,
  getMe,
  listWalls,
  requestMagicLink,
} from './client';

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('request', () => {
  it('resolves undefined for a 204 response', async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(deleteSession('loc-1', 'sess-1')).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/v1/locations/loc-1/sessions/sess-1');
    expect(init.method).toBe('DELETE');
    expect(init.credentials).toBe('same-origin');
  });

  it('parses a JSON response body', async () => {
    const walls = [{ id: 'w1', name: 'Slab' }];
    fetchMock.mockResolvedValueOnce(jsonResponse(200, walls));

    await expect(listWalls('loc-1')).resolves.toEqual(walls);
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/locations/loc-1/walls');
  });

  it('serializes a JSON request body with Content-Type', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(202, { ok: true }));

    await expect(requestMagicLink({ email: 'x@example.com' })).resolves.toEqual({
      ok: true,
    });

    const [, init] = fetchMock.mock.calls[0];
    expect(init.method).toBe('POST');
    expect(init.headers['Content-Type']).toBe('application/json');
    expect(JSON.parse(init.body)).toEqual({ email: 'x@example.com' });
  });

  it('throws ApiClientError with status + parsed body on non-2xx JSON', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(422, { error: 'grade is required' }));

    const err = await listWalls('loc-1').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    const apiErr = err as ApiClientError;
    expect(apiErr.status).toBe(422);
    expect(apiErr.body).toEqual({ error: 'grade is required' });
    expect(apiErr.message).toBe('grade is required');
  });

  it('keeps a non-JSON error body as raw text', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response('Internal Server Error', { status: 500 }),
    );

    const err = await listWalls('loc-1').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    const apiErr = err as ApiClientError;
    expect(apiErr.status).toBe(500);
    expect(apiErr.body).toBe('Internal Server Error');
    expect(apiErr.message).toBe('Internal Server Error');
  });
});

describe('getMe', () => {
  it('resolves null on 401 (a real logout, not an error)', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(401, { error: 'unauthorized' }));

    await expect(getMe()).resolves.toBeNull();
  });

  it('throws on any other failure (5xx must not look like a logout)', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(500, { error: 'db timeout' }));

    const err = await getMe().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    expect((err as ApiClientError).status).toBe(500);
  });
});
