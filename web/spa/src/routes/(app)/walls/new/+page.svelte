<script lang="ts">
  import { goto } from '$app/navigation';
  import {
    createWall,
    ApiClientError,
    type WallWriteShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import WallForm from '$lib/components/WallForm.svelte';

  const locId = $derived(effectiveLocationId());

  let saving = $state(false);
  let error = $state<string | null>(null);

  async function submit(body: WallWriteShape) {
    if (!locId) return;
    saving = true;
    error = null;
    try {
      const created = await createWall(locId, body);
      goto(`/walls/${created.id}`);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not create wall.';
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>New wall — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/walls">← Walls</a>
  <h1>New wall</h1>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar first.</p>
  {:else}
    <WallForm
      submitLabel="Create wall"
      onSubmit={submit}
      onCancel={() => goto('/walls')}
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
</style>
