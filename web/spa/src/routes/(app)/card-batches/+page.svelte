<script lang="ts">
  import {
    listCardBatches,
    cardBatchDownloadUrl,
    ApiClientError,
    type CardBatchShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  let batches = $state<CardBatchShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    listCardBatches(locId)
      .then((res) => {
        if (!cancelled) batches = res;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load card batches.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString();
  }

  function themeLabel(t: string): string {
    return t.replace(/_/g, ' ');
  }
</script>

<svelte:head>
  <title>Card batches — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Card batches</h1>
      <p class="lede">Print-and-cut PDFs for route cards. PDF re-renders from live data on every download.</p>
    </div>
    {#if locId}
      <a class="btn-primary" href="/card-batches/new">+ New batch</a>
    {/if}
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if loading}
    <p class="muted">Loading batches…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if batches.length === 0}
    <div class="empty-card">
      <h3>No batches yet</h3>
      <p>Create a batch to print route cards in bulk.</p>
      <a class="btn-primary" href="/card-batches/new">Create batch</a>
    </div>
  {:else}
    <ul class="batch-list">
      {#each batches as b (b.id)}
        <li>
          <a class="batch-card" href="/card-batches/{b.id}">
            <span class="batch-meta">
              <span class="status-pill status-{b.status}">{b.status}</span>
              <span class="theme">{themeLabel(b.theme)}</span>
            </span>
            <span class="batch-summary">
              {b.route_ids.length} route{b.route_ids.length === 1 ? '' : 's'}
              {#if b.page_count > 0}· {b.page_count} page{b.page_count === 1 ? '' : 's'}{/if}
            </span>
            <span class="created muted">{fmtDate(b.created_at)}</span>
          </a>
          {#if b.status !== 'failed'}
            <a class="dl-link" href={cardBatchDownloadUrl(locId, b.id)} target="_blank" rel="noreferrer">
              Download PDF
            </a>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .page {
    max-width: 60rem;
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
    max-width: 36rem;
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
    white-space: nowrap;
  }
  .btn-primary:hover {
    background: var(--rw-accent-hover);
  }
  .batch-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .batch-list li {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 12px;
    align-items: center;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 10px;
    padding: 0.7rem 1rem;
  }
  .batch-card {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 8px 16px;
    align-items: center;
    text-decoration: none;
    color: inherit;
  }
  .batch-meta {
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }
  .status-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .status-pending {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .status-ready {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-failed {
    background: #fef2f2;
    color: #b91c1c;
    border: 1px solid #fecaca;
  }
  .theme {
    color: var(--rw-text-muted);
    font-size: 0.8rem;
    text-transform: capitalize;
  }
  .batch-summary {
    color: var(--rw-text);
    font-weight: 500;
  }
  .created {
    font-size: 0.85rem;
  }
  .dl-link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
    font-size: 0.85rem;
    font-weight: 600;
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
