<script lang="ts">
  import {
    listMyNotifications,
    markNotificationRead,
    markAllNotificationsRead,
    ApiClientError,
    type NotificationShape,
  } from '$lib/api/client';

  let notifs = $state<NotificationShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let mutatingId = $state<number | null>(null);
  let allMarking = $state(false);

  $effect(() => {
    let cancelled = false;
    loading = true;
    error = null;
    listMyNotifications()
      .then((list) => {
        if (!cancelled) notifs = list;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load notifications.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function markOne(n: NotificationShape) {
    mutatingId = n.id;
    try {
      await markNotificationRead(n.id);
      notifs = notifs.filter((x) => x.id !== n.id);
    } catch (err) {
      // 404 = already read; treat as success and drop the row.
      if (err instanceof ApiClientError && err.status === 404) {
        notifs = notifs.filter((x) => x.id !== n.id);
      } else {
        error = err instanceof ApiClientError ? err.message : 'Could not mark read.';
      }
    } finally {
      mutatingId = null;
    }
  }

  async function markAll() {
    if (notifs.length === 0) return;
    allMarking = true;
    try {
      await markAllNotificationsRead();
      notifs = [];
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not mark all read.';
    } finally {
      allMarking = false;
    }
  }

  function fmtAge(iso: string): string {
    const ms = Date.now() - new Date(iso).getTime();
    const sec = Math.floor(ms / 1000);
    if (sec < 60) return `${sec}s ago`;
    const min = Math.floor(sec / 60);
    if (min < 60) return `${min}m ago`;
    const hr = Math.floor(min / 60);
    if (hr < 24) return `${hr}h ago`;
    const day = Math.floor(hr / 24);
    if (day < 7) return `${day}d ago`;
    return new Date(iso).toLocaleDateString();
  }
</script>

<svelte:head>
  <title>Notifications — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Notifications</h1>
      <p class="lede">Updates from your gyms — sends on your routes, new comps, session publications.</p>
    </div>
    {#if notifs.length > 0}
      <button class="ghost" disabled={allMarking} onclick={markAll}>
        {allMarking ? 'Marking…' : 'Mark all read'}
      </button>
    {/if}
  </header>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if notifs.length === 0}
    <div class="empty-card">
      <h3>You're all caught up</h3>
      <p>New notifications show up here as they happen.</p>
    </div>
  {:else}
    <ul class="notif-list">
      {#each notifs as n (n.id)}
        <li class="notif">
          <div class="head">
            <span class="type-pill type-{n.type}">{n.type.replace(/_/g, ' ')}</span>
            <span class="age muted">{fmtAge(n.created_at)}</span>
          </div>
          <div class="title">{n.title}</div>
          {#if n.body}<p class="body">{n.body}</p>{/if}
          <div class="actions">
            {#if n.link}
              <a class="link" href={n.link} onclick={() => markOne(n)}>Open →</a>
            {/if}
            <button
              class="ghost"
              disabled={mutatingId === n.id}
              onclick={() => markOne(n)}>
              {mutatingId === n.id ? '…' : 'Mark read'}
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .page {
    max-width: 48rem;
  }
  .page-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
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
  .notif-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .notif {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 10px;
    padding: 0.85rem 1rem;
  }
  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 6px;
  }
  .type-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .type-route_send,
  .type-route_flash {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .type-route_rated {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .type-session_published {
    background: rgba(59, 130, 246, 0.12);
    color: #1d4ed8;
  }
  .age {
    font-size: 0.8rem;
  }
  .title {
    font-weight: 600;
    color: var(--rw-text);
    margin-bottom: 4px;
  }
  .body {
    margin: 0 0 0.5rem;
    color: var(--rw-text);
    font-size: 0.92rem;
    line-height: 1.4;
  }
  .actions {
    display: flex;
    gap: 12px;
    align-items: center;
  }
  .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
    font-size: 0.85rem;
    font-weight: 600;
  }
  button {
    cursor: pointer;
    padding: 0.4rem 0.8rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.8rem;
    font-weight: 600;
  }
  button.ghost {
    background: transparent;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
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
