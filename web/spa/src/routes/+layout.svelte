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
    /* Neutrals — cool, near-neutral grays */
    --rw-bg:           #f6f7f9;
    --rw-surface:      #ffffff;
    --rw-surface-alt:  #eef0f3;
    --rw-border:       #e3e6ec;
    --rw-border-strong:#cdd2da;
    --rw-text:         #0f1422;
    --rw-text-muted:   #4b5567;
    --rw-text-faint:   #7a8497;

    /* Accent — electric lime */
    --rw-accent:       #c6f23c;
    --rw-accent-hover: #b3e02a;
    --rw-accent-ink:   #0f1422;

    /* Semantic */
    --rw-success:      #16a34a;
    --rw-warning:      #f59e0b;
    --rw-danger:       #ef4444;
    --rw-info:         #3b82f6;

    --rw-focus-ring:   rgba(198, 242, 60, 0.55);
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
