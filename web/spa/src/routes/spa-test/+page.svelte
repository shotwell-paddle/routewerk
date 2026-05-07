<script lang="ts">
  import { onMount } from 'svelte';

  let mounted = $state(false);
  let now = $state('');

  onMount(() => {
    mounted = true;
    now = new Date().toISOString();
  });
</script>

<svelte:head>
  <title>Routewerk SPA — smoke test</title>
</svelte:head>

<main>
  <h1>Routewerk SPA</h1>
  <p>Phase 0 smoke test. If you can see this, the SvelteKit build is embedded
    in the Go binary and routing through the chi mux is working.</p>
  <ul>
    <li>Hydrated: <strong>{mounted ? 'yes' : 'no'}</strong></li>
    <li>Loaded at: <code>{now || '—'}</code></li>
  </ul>
</main>

<style>
  main {
    font-family: system-ui, -apple-system, sans-serif;
    max-width: 40rem;
    margin: 4rem auto;
    padding: 0 1rem;
    color: #0f172a;
  }
  h1 { font-size: 1.5rem; }
  code { background: #f1f5f9; padding: 0 .25rem; border-radius: 3px; }
</style>
