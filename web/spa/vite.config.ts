import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [sveltekit()],
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
