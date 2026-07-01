import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [sveltekit()],
  test: {
    // Pure-logic suites only (API client, stores) — no DOM needed. The
    // sveltekit plugin above compiles runes in .svelte.ts imports.
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
  server: {
    port: 5173,
    strictPort: true,
    // During SPA dev, proxy /api to the local Go API so cookies + same-origin
    // assumptions hold. Override the target via VITE_API_TARGET if needed.
    proxy: {
      '/api': {
        target: process.env.VITE_API_TARGET ?? 'http://localhost:8080',
        changeOrigin: false,
      },
    },
  },
});
