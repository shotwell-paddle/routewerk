// Playwright config for the e2e smoke test (e2e/smoke.spec.ts).
//
// The test expects a FULLY BUILT server (spa_embed) already running and
// seeded — Playwright does not start it. See e2e/README.md for the local
// run recipe and .github/workflows/ci.yml (e2e-smoke job) for CI wiring.
//
// Deliberately separate from vitest: `npm run test` includes only
// src/**/*.test.ts (see vite.config.ts), and this config only looks at
// e2e/. Neither suite can pick up the other's files.

import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  // One spec, one worker — the flow is stateful (login → create), so
  // there's nothing to parallelize and no cross-test interference.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  // One retry in CI so a transient hiccup (slow cold start, GC pause on a
  // shared runner) doesn't fail the job; the retry records a trace for
  // debugging. Locally fail fast.
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI
    ? [['list'], ['html', { open: 'never' }]]
    : [['list']],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
