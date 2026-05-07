<script lang="ts">
  import {
    listSessions,
    ApiClientError,
    type SessionShape,
    type SessionStatus,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  let sessions = $state<SessionShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    listSessions(locId)
      .then((res) => {
        if (!cancelled) sessions = res;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load sessions.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  // Group by status; within each group sort newest scheduled date first.
  const grouped = $derived.by(() => {
    const out: Record<SessionStatus, SessionShape[]> = {
      planning: [],
      in_progress: [],
      complete: [],
      cancelled: [],
    };
    for (const s of sessions) {
      const bucket = out[s.status as SessionStatus] ?? out.planning;
      bucket.push(s);
    }
    for (const k of Object.keys(out) as SessionStatus[]) {
      out[k].sort(
        (a, b) =>
          new Date(b.scheduled_date).getTime() - new Date(a.scheduled_date).getTime(),
      );
    }
    return out;
  });

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
    });
  }

  const STATUS_LABEL: Record<SessionStatus, string> = {
    planning: 'Planning',
    in_progress: 'In progress',
    complete: 'Complete',
    cancelled: 'Cancelled',
  };
</script>

<svelte:head>
  <title>Sessions — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Setting sessions</h1>
      <p class="lede">Plan and track route-setting work at this location.</p>
    </div>
    {#if locId}
      <a class="btn-primary" href="/sessions/new">+ New session</a>
    {/if}
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if loading}
    <p class="muted">Loading sessions…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if sessions.length === 0}
    <div class="empty-card">
      <h3>No sessions yet</h3>
      <p>Schedule your first setting session to start planning routes.</p>
      <a class="btn-primary" href="/sessions/new">Create session</a>
    </div>
  {:else}
    {#each ['in_progress', 'planning', 'complete', 'cancelled'] as SessionStatus[] as status}
      {@const list = grouped[status]}
      {#if list.length > 0}
        <section class="group">
          <h2 class="group-head">
            <span class="status-pill status-{status}">{STATUS_LABEL[status]}</span>
            <span class="count">{list.length}</span>
          </h2>
          <ul class="session-list">
            {#each list as s (s.id)}
              <li>
                <a class="session-card" href="/sessions/{s.id}">
                  <span class="session-date">{fmtDate(s.scheduled_date)}</span>
                  <span class="session-meta muted">
                    {s.assignments?.length ?? 0} assignment{(s.assignments?.length ?? 0) === 1 ? '' : 's'}
                    {#if s.notes}· {s.notes.split('\n')[0].slice(0, 80)}{/if}
                  </span>
                  <span class="session-arrow">→</span>
                </a>
              </li>
            {/each}
          </ul>
        </section>
      {/if}
    {/each}
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
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
  }
  .btn-primary:hover {
    background: var(--rw-accent-hover);
  }
  .group {
    margin-bottom: 1.5rem;
  }
  .group-head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.95rem;
    margin: 0 0 0.6rem;
  }
  .count {
    color: var(--rw-text-faint);
    font-weight: 500;
    font-size: 0.85rem;
  }
  .status-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .status-planning {
    background: rgba(59, 130, 246, 0.12);
    color: #1d4ed8;
  }
  .status-in_progress {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .status-complete {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-cancelled {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .session-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .session-card {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 12px;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 10px;
    padding: 0.7rem 1rem;
    text-decoration: none;
    color: inherit;
    transition: border-color 120ms;
  }
  .session-card:hover {
    border-color: var(--rw-accent);
  }
  .session-date {
    font-weight: 600;
    color: var(--rw-text);
  }
  .session-meta {
    font-size: 0.85rem;
  }
  .session-arrow {
    color: var(--rw-text-faint);
    font-size: 1.1rem;
  }
  .session-card:hover .session-arrow {
    color: var(--rw-accent);
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
