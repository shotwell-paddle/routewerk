<script lang="ts">
  import {
    listRoutes,
    listWalls,
    ApiClientError,
    type RouteShape,
    type RouteType,
    type RouteStatus,
    type WallShape,
    type RouteListFilters,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';

  const PAGE_SIZE = 50;

  let routes = $state<RouteShape[]>([]);
  let total = $state(0);
  let walls = $state<WallShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let filters = $state({
    status: 'active' as RouteStatus | '',
    route_type: '' as RouteType | '',
    wall_id: '',
    grade: '',
    offset: 0,
  });

  const locId = $derived(effectiveLocationId());
  // Setter+ may create routes; the API enforces this server-side, the UI
  // just hides the affordance for climbers so they don't bounce off a 403.
  const canManage = $derived(roleRankAt(locId) >= 2);

  // Build the wall lookup once routes load — the route card needs the wall
  // name, which the route shape doesn't include directly.
  const wallNameById = $derived.by(() => {
    const m = new Map<string, string>();
    for (const w of walls) m.set(w.id, w.name);
    return m;
  });

  // Walls list (for the wall filter dropdown). One-shot per location change.
  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    listWalls(locId).then((res) => {
      if (!cancelled) walls = res;
    });
    return () => {
      cancelled = true;
    };
  });

  // Routes list — refetch any time the location or filters change.
  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    const f: RouteListFilters = {
      limit: PAGE_SIZE,
      offset: filters.offset,
    };
    if (filters.status) f.status = filters.status;
    if (filters.route_type) f.route_type = filters.route_type;
    if (filters.wall_id) f.wall_id = filters.wall_id;
    if (filters.grade) f.grade = filters.grade;
    listRoutes(locId, f)
      .then((res) => {
        if (cancelled) return;
        routes = res.routes;
        total = res.total;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load routes.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function onFilterChange() {
    // Always reset paging when a filter changes.
    filters.offset = 0;
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString();
  }

  function stars(avg: number): string {
    if (avg <= 0) return '—';
    const full = Math.round(avg);
    return '★'.repeat(full) + '☆'.repeat(Math.max(0, 5 - full));
  }

  const showingStart = $derived(routes.length > 0 ? filters.offset + 1 : 0);
  const showingEnd = $derived(filters.offset + routes.length);
  const hasPrev = $derived(filters.offset > 0);
  const hasNext = $derived(filters.offset + routes.length < total);
</script>

<svelte:head>
  <title>Routes — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Routes</h1>
      <p class="lede">Browse the active route list for the selected location.</p>
    </div>
    {#if locId && canManage}
      <a class="btn-primary" href="/routes/new">+ New route</a>
    {/if}
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else}
    <div class="filter-bar">
      <label>
        <span>Status</span>
        <select bind:value={filters.status} onchange={onFilterChange}>
          <option value="active">Active</option>
          <option value="flagged">Flagged</option>
          <option value="archived">Archived</option>
          <option value="">All</option>
        </select>
      </label>
      <label>
        <span>Type</span>
        <select bind:value={filters.route_type} onchange={onFilterChange}>
          <option value="">Any</option>
          <option value="boulder">Boulder</option>
          <option value="route">Route</option>
        </select>
      </label>
      <label>
        <span>Wall</span>
        <select bind:value={filters.wall_id} onchange={onFilterChange}>
          <option value="">Any</option>
          {#each walls as w (w.id)}
            <option value={w.id}>{w.name}</option>
          {/each}
        </select>
      </label>
      <label class="grade-input">
        <span>Grade</span>
        <input
          type="text"
          bind:value={filters.grade}
          onchange={onFilterChange}
          placeholder="e.g. V4 or 5.11a" />
      </label>
    </div>

    {#if loading && routes.length === 0}
      <p class="muted">Loading routes…</p>
    {:else if error}
      <p class="error">{error}</p>
    {:else if routes.length === 0}
      <div class="empty-card">
        <h3>No routes match these filters</h3>
        <p>Try widening the status or clearing the wall filter.</p>
      </div>
    {:else}
      <div class="results-meta muted">
        Showing {showingStart}–{showingEnd} of {total}
        {loading ? ' · loading…' : ''}
      </div>
      <ul class="route-list">
        {#each routes as route (route.id)}
          <li>
            <a class="route-card" href="/routes/{route.id}">
              <span
                class="color-chip"
                style="background:{route.color}"
                title={route.color}></span>
              <span class="primary">
                <span class="grade">{route.grade}</span>
                {#if route.name}<span class="name">{route.name}</span>{/if}
                <span class="status status-{route.status}">{route.status}</span>
              </span>
              <span class="meta-line muted">
                <span>{wallNameById.get(route.wall_id) ?? '—'}</span>
                <span>·</span>
                <span>{route.route_type}</span>
                <span>·</span>
                <span title="Set">{fmtDate(route.date_set)}</span>
              </span>
              <span class="stats muted">
                <span title="Average rating">{stars(route.avg_rating)}</span>
                <span>·</span>
                <span>{route.ascent_count} sends</span>
              </span>
            </a>
          </li>
        {/each}
      </ul>

      {#if hasPrev || hasNext}
        <div class="pager">
          <button
            disabled={!hasPrev || loading}
            onclick={() => (filters.offset = Math.max(0, filters.offset - PAGE_SIZE))}>
            ← Prev
          </button>
          <button
            disabled={!hasNext || loading}
            onclick={() => (filters.offset = filters.offset + PAGE_SIZE)}>
            Next →
          </button>
        </div>
      {/if}
    {/if}
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
    margin-bottom: 1.25rem;
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
  }
  .btn-primary:hover {
    background: var(--rw-accent-hover);
  }
  .filter-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 0.85rem;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 0.85rem 1rem;
    margin-bottom: 1rem;
  }
  .filter-bar label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--rw-text-muted);
    font-weight: 600;
    min-width: 9rem;
    flex: 1 1 9rem;
  }
  .filter-bar select,
  .filter-bar input {
    padding: 0.45rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .grade-input {
    flex: 0.7 1 8rem;
  }
  .results-meta {
    margin-bottom: 0.75rem;
    font-size: 0.85rem;
  }
  .route-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .route-card {
    display: grid;
    grid-template-columns: 36px 1fr auto;
    grid-template-areas:
      'color primary stats'
      'color meta stats';
    column-gap: 12px;
    row-gap: 2px;
    align-items: center;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 10px;
    padding: 0.7rem 1rem;
    text-decoration: none;
    color: inherit;
    transition: border-color 120ms;
  }
  .route-card:hover {
    border-color: var(--rw-accent);
  }
  .color-chip {
    grid-area: color;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .primary {
    grid-area: primary;
    display: inline-flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }
  .grade {
    font-weight: 700;
    color: var(--rw-text);
    font-size: 1.05rem;
  }
  .name {
    color: var(--rw-text);
    font-size: 0.95rem;
  }
  .status {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 1px 7px;
    border-radius: 4px;
    font-weight: 700;
  }
  .status-active {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-flagged {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .status-archived {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .meta-line {
    grid-area: meta;
    font-size: 0.82rem;
    display: inline-flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .stats {
    grid-area: stats;
    display: inline-flex;
    gap: 6px;
    font-size: 0.85rem;
    color: var(--rw-text-muted);
    align-items: center;
  }
  .pager {
    display: flex;
    justify-content: space-between;
    margin-top: 1rem;
  }
  button {
    cursor: pointer;
    padding: 0.5rem 1rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.9rem;
    font-weight: 600;
  }
  button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .empty-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 2rem 1.5rem;
    text-align: center;
  }
  .empty-card h3 {
    margin: 0 0 0.4rem;
  }
  .empty-card p {
    color: var(--rw-text-muted);
    margin: 0;
  }
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
</style>
