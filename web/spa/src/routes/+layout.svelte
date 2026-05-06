<script lang="ts">
  import { onMount } from 'svelte';
  import { loadMe } from '$lib/stores/auth.svelte';

  let { children } = $props();

  // Kick off the /me fetch as soon as the SPA mounts. Components that
  // need the current user can read from `authState()`; the store
  // dedupes parallel callers so this is safe to call from anywhere.
  onMount(() => {
    loadMe();
  });
</script>

{@render children()}

<style>
  :global(:root) {
    /* Neutrals — warm, matching the HTMX shell at web/static/css/routewerk.css. */
    --rw-bg:           #f7f7f5;
    --rw-surface:      #ffffff;
    --rw-surface-alt:  #efeeeb;
    --rw-border:       #e8e6e1;
    --rw-border-strong:#d4d1c9;
    --rw-text:         #1c1b18;
    --rw-text-muted:   #5b5751;
    --rw-text-faint:   #9b9590;

    /* Accent — Strava-orange nod from the HTMX brand. */
    --rw-accent:       #fc5200;
    --rw-accent-hover: #e04800;
    --rw-accent-ink:   #ffffff;

    /* Semantic */
    --rw-success:      #16a34a;
    --rw-warning:      #f59e0b;
    --rw-danger:       #ef4444;
    --rw-info:         #1565c0;

    --rw-focus-ring:   rgba(252, 82, 0, 0.45);
  }

  :global(body) {
    margin: 0;
    font-family:
      ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto,
      'Helvetica Neue', Arial, sans-serif;
    color: var(--rw-text);
    background: var(--rw-bg);
    -webkit-font-smoothing: antialiased;
  }

  :global(*:focus-visible) {
    outline: 2px solid var(--rw-accent);
    outline-offset: 2px;
  }
</style>
