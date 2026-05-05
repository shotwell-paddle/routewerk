import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

const config = {
  preprocess: vitePreprocess(),

  kit: {
    // SPA mode: single fallback HTML, client-side routing handles everything.
    // The Go server mounts this build under one or more URL prefixes
    // (e.g. /spa-test/* in Phase 0; /comp/*, /staff/comp/* in Phase 1).
    adapter: adapter({
      pages: 'build',
      assets: 'build',
      fallback: 'index.html',
      precompress: false,
      strict: false,
    }),
    // Relative asset paths so the same build works under any URL prefix.
    paths: {
      relative: true,
    },
  },
};

export default config;
