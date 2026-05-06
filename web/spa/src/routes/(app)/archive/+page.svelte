<script lang="ts">
  import {
    listRoutes,
    listWalls,
    ApiClientError,
    type RouteShape,
    type WallShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  // Climber-flavored read-only view of archived routes for the selected
  // location. Mirrors the HTMX `/archive` page (climber/archive.html) —
  // browsable history of "what used to be on the wall" sorted by most
  // recently stripped first. Staff manage actions live on /routes;
  // this page never shows them.

  let routes = $state<RouteShape[]>([]);
  let walls = $state<WallShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let typeFilter = $state<'' | 'boulder' | 'route'>('');
  let wallFilter = $state('');

  const locId = $derived(effectiveLocationId());

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      listRoutes(locId, {
        status: 'archived',
        route_type: typeFilter || undefined,
        wall_id: wallFilter || undefined,
        limit: 100,
      }),
      walls.length === 0
        ? listWalls(locId).catch(() => [] as WallShape[])
        : Promise.resolve(walls),
    ])
      .then(([res, w]) => {
        if (cancelled) return;
        // Sort descending by date_stripped if present, else by date_set.
        // The server doesn't guarantee an order on archived routes, but
        // climbers want "most recent first" here.
        routes = [...res.routes].sort((a, b) => {
          const ka = a.date_stripped ?? a.date_set;
          const kb = b.date_stripped ?? b.date_set;
          return kb.localeCompare(ka);
        });
        walls = w;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load archive.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function fmtDate(iso: string | null | undefined): string {
    if (!iso) return '—';
    return new Date(iso).toLocaleDateString();
  }

  function wallName(id: string): string {
    return walls.find((w) => w.id === id)?.name ?? '—';
  }

  function displayGrade(r: RouteShape): string {
    if (r.grading_system === 'circuit') return r.circuit_color ?? r.grade;
    return r.grade;
  }
</script>

<svelte:head>
  <title>Archive — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <h1>Archive</h1>
    <p class="lede">Routes that have come down recently. Most recently stripped first.</p>
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar to see its archive.</p>
  {:else}
    <div class="filter-bar">
      <label>
        <span>Type</span>
        <select bind:value={typeFilter}>
          <option value="">Any</option>
          <option value="boulder">Boulder</option>
          <option value="route">Route</option>
        </select>
      </label>
      <label>
        <span>Wall</span>
        <select bind:value={wallFilter}>
          <option value="">Any</option>
          {#each walls as w (w.id)}
            <option value={w.id}>{w.name}</option>
          {/each}
        </select>
      </label>
    </div>

    {#if error}<p class="error">{error}</p>{/if}

    {#if loading && routes.length === 0}
      <p class="muted">Loading archive…</p>
    {:else if routes.length === 0}
      <div class="empty-card">
        <h3>Nothing in the archive yet</h3>
        <p>When routes are stripped from the wall, they show up here.</p>
      </div>
    {:else}
      <ul class="route-list">
        {#each routes as r (r.id)}
          <li>
            <a class="route-row" href="/routes/{r.id}">
              <span class="color-chip" style="background:{r.color}"></span>
              <span class="grade">{displayGrade(r)}</span>
              <span class="rname">{r.name ?? '—'}</span>
              <span class="meta muted">
                {wallName(r.wall_id)} ·
                {#if r.date_stripped}
                  stripped {fmtDate(r.date_stripped)}
                {:else}
                  set {fmtDate(r.date_set)}
                {/if}
              </span>
              {#if r.ascent_count > 0}
                <span class="ascents muted">{r.ascent_count} sends</span>
              {/if}
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
  }
  .page-header {
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
  .filter-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 1rem;
  }
  .filter-bar label {
    display: flex;
    flex-direction: column;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--rw-text-muted);
    font-weight: 600;
    gap: 4px;
  }
  .filter-bar select {
    padding: 0.4rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.9rem;
    text-transform: none;
    letter-spacing: 0;
    font-weight: 500;
  }
  .route-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .route-row {
    display: grid;
    grid-template-columns: 18px 4rem 1fr auto auto;
    align-items: center;
    gap: 12px;
    padding: 0.55rem 0.85rem;
    border: 1px solid var(--rw-border);
    border-radius: 8px;
    background: var(--rw-surface);
    text-decoration: none;
    color: inherit;
  }
  .route-row:hover {
    border-color: var(--rw-accent);
  }
  .color-chip {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .grade {
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .rname {
    color: var(--rw-text);
    font-weight: 500;
  }
  .meta,
  .ascents {
    font-size: 0.85rem;
  }
  .muted {
    color: var(--rw-text-muted);
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
    margin: 0;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
</style>
