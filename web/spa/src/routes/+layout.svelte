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
  :global(body) {
    margin: 0;
    font-family: system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
    color: #0f172a;
    background: #fff;
  }
</style>
