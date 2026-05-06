<script lang="ts">
  import { goto } from '$app/navigation';
  import {
    listRoutes,
    listWalls,
    createCardBatch,
    CARD_THEMES,
    CUTTER_PROFILES,
    ApiClientError,
    type RouteShape,
    type WallShape,
    type CardTheme,
    type CutterProfile,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  // Pull a generous window of active routes so the picker is useful
  // without paginating. The handler caps batch size at MaxBatchCards
  // (matches the HTMX form), so we don't need every route.
  const ROUTE_WINDOW = 200;

  const locId = $derived(effectiveLocationId());

  let routes = $state<RouteShape[]>([]);
  let walls = $state<WallShape[]>([]);
  let loading = $state(true);
  let loadError = $state<string | null>(null);

  let saving = $state(false);
  let saveError = $state<string | null>(null);

  let theme = $state<CardTheme>('trading_card');
  let cutter = $state<CutterProfile>('silhouette_type2');
  let wallFilter = $state('');
  let selected = $state<Set<string>>(new Set());

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    loadError = null;
    Promise.all([
      listRoutes(locId, { status: 'active', limit: ROUTE_WINDOW }),
      listWalls(locId),
    ])
      .then(([rs, ws]) => {
        if (cancelled) return;
        routes = rs.routes;
        walls = ws;
      })
      .catch((err) => {
        if (cancelled) return;
        loadError = err instanceof ApiClientError ? err.message : 'Could not load routes.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  const wallNameById = $derived.by(() => {
    const m = new Map<string, string>();
    for (const w of walls) m.set(w.id, w.name);
    return m;
  });

  const visible = $derived.by(() => {
    if (!wallFilter) return routes;
    return routes.filter((r) => r.wall_id === wallFilter);
  });

  function toggle(id: string) {
    if (selected.has(id)) selected.delete(id);
    else selected.add(id);
    selected = new Set(selected);
  }

  function selectAll() {
    const next = new Set(selected);
    for (const r of visible) next.add(r.id);
    selected = next;
  }

  function clearSelection() {
    selected = new Set();
  }

  async function submit() {
    if (!locId) return;
    if (selected.size === 0) {
      saveError = 'Pick at least one route.';
      return;
    }
    saving = true;
    saveError = null;
    try {
      const created = await createCardBatch(locId, {
        route_ids: Array.from(selected),
        theme,
        cutter_profile: cutter,
      });
      goto(`/app/card-batches/${created.id}`);
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Could not create batch.';
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>New card batch — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/card-batches">← Card batches</a>
  <h1>New card batch</h1>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar first.</p>
  {:else if loading}
    <p class="muted">Loading active routes…</p>
  {:else if loadError}
    <p class="error">{loadError}</p>
  {:else}
    <section class="settings card">
      <div class="row">
        <label>
          <span>Theme</span>
          <select bind:value={theme}>
            {#each CARD_THEMES as t}<option value={t.value}>{t.label}</option>{/each}
          </select>
        </label>
        <label>
          <span>Cutter</span>
          <select bind:value={cutter}>
            {#each CUTTER_PROFILES as c}<option value={c.value}>{c.label}</option>{/each}
          </select>
        </label>
      </div>
    </section>

    <section class="card">
      <div class="picker-head">
        <div>
          <h2>Routes</h2>
          <p class="muted picker-meta">
            {selected.size} selected of {visible.length} shown
            {#if routes.length === ROUTE_WINDOW}· (showing first {ROUTE_WINDOW}){/if}
          </p>
        </div>
        <div class="picker-actions">
          <label class="filter">
            <span>Wall</span>
            <select bind:value={wallFilter}>
              <option value="">Any</option>
              {#each walls as w (w.id)}
                <option value={w.id}>{w.name}</option>
              {/each}
            </select>
          </label>
          <button onclick={selectAll}>Select all visible</button>
          <button onclick={clearSelection} disabled={selected.size === 0}>Clear</button>
        </div>
      </div>

      {#if visible.length === 0}
        <p class="muted">No active routes match the wall filter.</p>
      {:else}
        <ul class="route-list">
          {#each visible as r (r.id)}
            <li>
              <label class="route-row">
                <input
                  type="checkbox"
                  checked={selected.has(r.id)}
                  onchange={() => toggle(r.id)} />
                <span class="color-chip" style="background:{r.color}"></span>
                <span class="grade">{r.grade}</span>
                {#if r.name}<span class="rname">{r.name}</span>{/if}
                <span class="rmeta muted">{wallNameById.get(r.wall_id) ?? '—'}</span>
              </label>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    {#if saveError}<p class="error">{saveError}</p>{/if}

    <div class="actions">
      <button class="primary" disabled={saving || selected.size === 0} onclick={submit}>
        {saving ? 'Creating…' : `Create batch (${selected.size})`}
      </button>
      <a class="cancel" href="/app/card-batches">Cancel</a>
    </div>
  {/if}
</div>

<style>
  .page {
    max-width: 60rem;
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
  h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.85rem;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--rw-text-muted);
  }
  select {
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .picker-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 0.85rem;
    flex-wrap: wrap;
  }
  .picker-meta {
    margin: 4px 0 0;
    font-size: 0.85rem;
  }
  .picker-actions {
    display: flex;
    align-items: flex-end;
    gap: 8px;
    flex-wrap: wrap;
  }
  .filter {
    flex-direction: row;
    align-items: center;
    gap: 6px;
    font-size: 0.8rem;
  }
  .filter select {
    min-width: 10rem;
  }
  .route-list {
    list-style: none;
    padding: 0;
    margin: 0;
    max-height: 30rem;
    overflow-y: auto;
    border: 1px solid var(--rw-border);
    border-radius: 8px;
  }
  .route-list li {
    border-bottom: 1px solid var(--rw-border);
  }
  .route-list li:last-child {
    border-bottom: none;
  }
  .route-row {
    display: grid;
    grid-template-columns: auto 18px 3rem 1fr auto;
    align-items: center;
    gap: 8px;
    padding: 0.5rem 0.85rem;
    cursor: pointer;
    font-size: 0.9rem;
    color: var(--rw-text);
    font-weight: 500;
  }
  .route-row:hover {
    background: var(--rw-surface-alt);
  }
  .color-chip {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .grade {
    font-weight: 700;
  }
  .rname {
    color: var(--rw-text);
  }
  .rmeta {
    font-size: 0.85rem;
  }
  .actions {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-top: 0.5rem;
  }
  button,
  .cancel {
    cursor: pointer;
    padding: 0.55rem 1rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.9rem;
    font-weight: 600;
    text-decoration: none;
    display: inline-block;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
  }
  button.primary:hover:not(:disabled) {
    background: var(--rw-accent-hover);
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.5rem 0;
  }
</style>
