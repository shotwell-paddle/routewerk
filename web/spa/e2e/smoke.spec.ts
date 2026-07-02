// E2E smoke: login → dashboard → create route → route appears.
//
// This is the one browser test that covers what unit tests can't: the
// cookie session + CSRF double-submit on the server-rendered /login form,
// the SPA booting against the real embedded build, the auth-gate redirect
// round-trip, and a real setter-gated API write (route creation).
//
// Prereqs (see e2e/README.md for the full local recipe):
//   1. Server built with -tags=spa_embed and running against a SCRATCH
//      database (migrations auto-run on startup).
//   2. Seed fixture applied: `go run ./web/spa/e2e/seed` (from repo root).
//      It creates the user below with a setter membership, one location,
//      and one boulder wall named "E2E Wall".
//   3. E2E_BASE_URL pointing at the server (default http://localhost:8080).
//
// The route name is unique per run so the test is re-runnable against the
// same scratch DB without ambiguity in the final list assertion.

import { test, expect } from '@playwright/test';

const EMAIL = 'e2e-setter@routewerk.test';
const PASSWORD = 'e2e-smoke-password';
const ROUTE_NAME = `E2E Smoke ${Date.now()}`;

test('login → dashboard → create route → route appears', async ({ page }) => {
  // ── 1. Unauthenticated visit bounces to the password login ──────
  // The SPA's (app) layout fires /api/v1/me; a real 401 redirects to the
  // server-rendered /login with ?next= preserving the target.
  await page.goto('/');
  await page.waitForURL(/\/login/);
  await expect(page.getByLabel('Email')).toBeVisible();

  // ── 2. Password login (cookie session + CSRF double-submit) ─────
  // The form carries a hidden _csrf_token that must match the CSRF
  // cookie; success mints the _rw_session cookie and 303s back to next.
  await page.getByLabel('Email').fill(EMAIL);
  await page.getByLabel('Password').fill(PASSWORD);
  await page.getByRole('button', { name: 'Sign In' }).click();

  // ── 3. SPA dashboard renders for a setter ────────────────────────
  await page.waitForURL((url) => url.pathname === '/');
  await expect(page.getByRole('heading', { name: /Welcome back/ })).toBeVisible();
  // The stat panel is setter-gated server-side (GET /locations/{id}/dashboard),
  // so its presence proves the cookie authenticated an API request too.
  await expect(page.getByText('Active routes', { exact: true })).toBeVisible();

  // ── 4. Create a route through the SPA form ───────────────────────
  await page.getByRole('link', { name: 'Routes', exact: true }).click();
  await page.getByRole('link', { name: '+ New route' }).click();
  await page.waitForURL(/\/routes\/new/);

  await page.getByLabel('Wall *').selectOption({ label: 'E2E Wall' });
  // Defaults: type=boulder, grading system=v_scale → V-grade chips.
  await page.getByRole('button', { name: 'V3', exact: true }).click();
  // Hold color swatches carry the gym palette name as their accessible name.
  await page.getByRole('button', { name: 'Blue', exact: true }).click();
  await page.getByLabel('Name', { exact: true }).fill(ROUTE_NAME);
  await page.getByRole('button', { name: 'Create route' }).click();

  // POST /locations/{id}/routes → client-side goto(/routes/{id}).
  await page.waitForURL(/\/routes\/[0-9a-f-]{36}$/);
  await expect(page.getByText(ROUTE_NAME)).toBeVisible();

  // ── 5. The new route appears in the routes list ──────────────────
  await page.goto('/routes');
  await expect(page.getByText(ROUTE_NAME)).toBeVisible();
});
