<script lang="ts">
  import { listWalls, ApiClientError, type WallShape } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  let walls = $state<WallShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());

  // Re-fetch any time the user picks a different location from the sidebar.
  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    listWalls(locId)
      .then((res) => {
        if (!cancelled) walls = res;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load walls.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });
</script>

<svelte:head>
  <title>Walls — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Walls</h1>
      <p class="lede">Manage the physical walls climbers route on.</p>
    </div>
    {#if locId}
      <a class="btn-primary" href="/app/walls/new">+ New wall</a>
    {/if}
  </header>

  {#if !locId}
    <p class="empty">Pick a location from the sidebar to see its walls.</p>
  {:else if loading}
    <p class="muted">Loading walls…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if walls.length === 0}
    <div class="empty-card">
      <h3>No walls yet</h3>
      <p>Create your first wall to start adding routes.</p>
      <a class="btn-primary" href="/app/walls/new">Create wall</a>
    </div>
  {:else}
    <div class="grid">
      {#each walls as w (w.id)}
        <a class="card" href="/app/walls/{w.id}">
          <div class="card-head">
            <span class="type-badge type-{w.wall_type}">{w.wall_type}</span>
            <h3 class="name">{w.name}</h3>
          </div>
          <dl class="meta">
            {#if w.angle}<div><dt>Angle</dt><dd>{w.angle}</dd></div>{/if}
            {#if w.height_meters != null}
              <div><dt>Height</dt><dd>{w.height_meters.toFixed(1)} m</dd></div>
            {/if}
            {#if w.num_anchors != null}
              <div><dt>Anchors</dt><dd>{w.num_anchors}</dd></div>
            {/if}
            {#if w.surface_type}<div><dt>Surface</dt><dd>{w.surface_type}</dd></div>{/if}
          </dl>
          <span class="card-cta">Manage →</span>
        </a>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page {
    max-width: 72rem;
  }
  .page-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
    letter-spacing: -0.01em;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
  }
  .btn-primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    padding: 0.55rem 1rem;
    border-radius: 8px;
    text-decoration: none;
    font-weight: 600;
    font-size: 0.9rem;
    border: 1px solid var(--rw-accent);
    transition: background 120ms;
  }
  .btn-primary:hover {
    background: var(--rw-accent-hover);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: 1rem;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1rem 1.1rem;
    text-decoration: none;
    color: inherit;
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    transition: border-color 120ms, transform 120ms;
  }
  .card:hover {
    border-color: var(--rw-accent);
    transform: translateY(-1px);
  }
  .card-head {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .type-badge {
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    color: var(--rw-text-muted);
    background: var(--rw-surface-alt);
  }
  .type-boulder {
    background: rgba(245, 158, 11, 0.15);
    color: #92590a;
  }
  .type-route {
    background: rgba(59, 130, 246, 0.12);
    color: #1d4ed8;
  }
  .name {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0;
    flex: 1;
  }
  .meta {
    margin: 0;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 4px 12px;
    color: var(--rw-text-muted);
    font-size: 0.85rem;
  }
  .meta dt {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--rw-text-faint);
    margin: 0;
  }
  .meta dd {
    margin: 0;
    color: var(--rw-text);
    font-weight: 500;
  }
  .card-cta {
    color: var(--rw-text-faint);
    font-size: 0.85rem;
    font-weight: 600;
    margin-top: auto;
  }
  .card:hover .card-cta {
    color: var(--rw-accent);
  }
  .empty,
  .muted {
    color: var(--rw-text-muted);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
  .empty-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 2.5rem 1.5rem;
    text-align: center;
  }
  .empty-card h3 {
    margin: 0 0 0.4rem;
    font-size: 1.15rem;
  }
  .empty-card p {
    color: var(--rw-text-muted);
    margin: 0 0 1.25rem;
  }
</style>
