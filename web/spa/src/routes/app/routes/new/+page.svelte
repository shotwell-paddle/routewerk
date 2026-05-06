<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    createRoute,
    listWalls,
    getLocationSettings,
    ApiClientError,
    type RouteWriteShape,
    type WallShape,
    type LocationSettingsShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import RouteForm from '$lib/components/RouteForm.svelte';

  const locId = $derived(effectiveLocationId());
  let walls = $state<WallShape[]>([]);
  // Settings are best-effort: a setter without head_setter+ won't be able
  // to PUT them, but the GET is open to setter+ so the form can mirror the
  // gym's preferences (allowed grading systems, circuit + hold colors,
  // strip-age default).
  let settings = $state<LocationSettingsShape | null>(null);
  let saving = $state(false);
  let error = $state<string | null>(null);

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    listWalls(locId).then((res) => {
      if (!cancelled) walls = res;
    });
    getLocationSettings(locId)
      .then((s) => {
        if (!cancelled) settings = s;
      })
      .catch(() => {
        // Permission or fetch error — RouteForm falls back to defaults.
      });
    return () => {
      cancelled = true;
    };
  });

  async function submit(body: RouteWriteShape) {
    if (!locId) return;
    saving = true;
    error = null;
    try {
      const created = await createRoute(locId, body);
      goto(`/app/routes/${created.id}`);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not create route.';
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>New route — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/routes">← Routes</a>
  <h1>New route</h1>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar first.</p>
  {:else if walls.length === 0}
    <p class="muted">
      No walls yet — <a class="link" href="/app/walls/new">create one first</a>.
    </p>
  {:else}
    <RouteForm
      {walls}
      {settings}
      submitLabel="Create route"
      onSubmit={submit}
      onCancel={() => goto('/app/routes')}
      {saving}
      {error} />
  {/if}
</div>

<style>
  .page {
    max-width: 48rem;
  }
  .back {
    display: inline-block;
    color: var(--rw-text-muted);
    text-decoration: none;
    font-size: 0.9rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }
  .back:hover {
    color: var(--rw-accent);
  }
  h1 {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0 0 1.25rem;
    letter-spacing: -0.01em;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
  }
</style>
