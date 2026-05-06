<script lang="ts">
  import {
    listMyAscents,
    getMyStats,
    updateMyAscent,
    deleteMyAscent,
    ApiClientError,
    type AscentWithRouteShape,
    type AscentType,
    type MyStatsShape,
  } from '$lib/api/client';
  import { currentUser } from '$lib/stores/auth.svelte';

  const PAGE_SIZE = 25;

  let ascents = $state<AscentWithRouteShape[]>([]);
  let total = $state(0);
  let stats = $state<MyStatsShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let offset = $state(0);

  const me = $derived(currentUser());

  $effect(() => {
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      listMyAscents(PAGE_SIZE, offset),
      getMyStats().catch(() => null),
    ])
      .then(([res, st]) => {
        if (cancelled) return;
        ascents = res.ascents ?? [];
        total = res.total;
        stats = st;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load profile data.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function fmtDateTime(iso: string): string {
    return new Date(iso).toLocaleString();
  }

  // Per-tick edit state. Only one row open at a time keeps the markup
  // simple; switching to another row closes the prior form.
  let editingId = $state<string | null>(null);
  let editForm = $state<{ ascent_type: AscentType; attempts: number; notes: string }>({
    ascent_type: 'send',
    attempts: 1,
    notes: '',
  });
  let editSaving = $state(false);
  let editError = $state<string | null>(null);
  let mutatingId = $state<string | null>(null);

  function openEdit(a: AscentWithRouteShape) {
    editingId = a.id;
    editForm = {
      ascent_type: (a.ascent_type as AscentType) ?? 'send',
      attempts: a.attempts ?? 1,
      notes: a.notes ?? '',
    };
    editError = null;
  }

  async function saveEdit(id: string) {
    if (editSaving) return;
    editSaving = true;
    editError = null;
    try {
      await updateMyAscent(id, {
        ascent_type: editForm.ascent_type,
        attempts: Math.max(1, Math.floor(editForm.attempts)),
        notes: editForm.notes.trim() || null,
      });
      // Reflect locally without a full re-fetch — the row stays in the
      // same position, the new fields render immediately.
      ascents = ascents.map((a) =>
        a.id === id
          ? {
              ...a,
              ascent_type: editForm.ascent_type,
              attempts: Math.max(1, Math.floor(editForm.attempts)),
              notes: editForm.notes.trim() || null,
            }
          : a,
      );
      editingId = null;
    } catch (err) {
      editError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      editSaving = false;
    }
  }

  async function removeAscent(id: string) {
    if (!confirm('Delete this tick? Your stats will update; the route stays.')) return;
    mutatingId = id;
    try {
      await deleteMyAscent(id);
      ascents = ascents.filter((a) => a.id !== id);
      total = Math.max(0, total - 1);
      // Refresh stats so the totals on the avatar row stay honest.
      try {
        stats = await getMyStats();
      } catch {
        // best-effort
      }
    } catch (err) {
      editError = err instanceof ApiClientError ? err.message : 'Delete failed.';
    } finally {
      mutatingId = null;
    }
  }

  const showingStart = $derived(ascents.length > 0 ? offset + 1 : 0);
  const showingEnd = $derived(offset + ascents.length);
  const hasPrev = $derived(offset > 0);
  const hasNext = $derived(offset + ascents.length < total);

  // Pyramid bars are scaled to the largest single-grade count.
  const pyramidMax = $derived(
    Math.max(1, ...(stats?.grade_pyramid?.map((p) => p.count) ?? [0])),
  );
</script>

<svelte:head>
  <title>Profile — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div class="who">
      {#if me?.avatar_url}
        <img class="avatar" src={me.avatar_url} alt="" />
      {:else}
        <div class="avatar avatar-fallback">{me?.display_name?.[0]?.toUpperCase() ?? '?'}</div>
      {/if}
      <div>
        <h1>{me?.display_name ?? '—'}</h1>
        <p class="lede">{me?.email ?? ''}</p>
        {#if me?.bio}<p class="bio">{me.bio}</p>{/if}
      </div>
    </div>
    <a class="settings-link" href="/settings">Edit profile →</a>
  </header>

  {#if error}
    <p class="error">{error}</p>
  {/if}

  {#if stats}
    <section class="stats-row">
      <div class="stat">
        <div class="stat-value">{stats.total_sends}</div>
        <div class="stat-label">Sends</div>
      </div>
      <div class="stat">
        <div class="stat-value">{stats.total_flashes}</div>
        <div class="stat-label">Flashes</div>
      </div>
      <div class="stat">
        <div class="stat-value">{stats.unique_routes}</div>
        <div class="stat-label">Unique routes</div>
      </div>
      <div class="stat">
        <div class="stat-value">{stats.total_logged}</div>
        <div class="stat-label">Total logged</div>
      </div>
    </section>

    {#if stats.grade_pyramid && stats.grade_pyramid.length > 0}
      <section class="card">
        <h2>Grade pyramid</h2>
        <div class="pyramid">
          {#each stats.grade_pyramid as entry}
            <div class="pyr-row">
              <span class="pyr-grade">{entry.grade}</span>
              <div class="pyr-bar">
                <span class="pyr-fill" style="width: {(entry.count / pyramidMax) * 100}%"></span>
              </div>
              <span class="pyr-count">{entry.count}</span>
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/if}

  <section class="card">
    <h2>Recent ticks</h2>
    {#if loading && ascents.length === 0}
      <p class="muted">Loading…</p>
    {:else if ascents.length === 0}
      <p class="muted">No ticks logged yet. Send something to start your log.</p>
    {:else}
      <div class="results-meta muted">Showing {showingStart}–{showingEnd} of {total}</div>
      <ul class="ascent-list">
        {#each ascents as a (a.id)}
          <li class:editing={editingId === a.id}>
            <span class="color-chip" style="background:{a.route_color}"></span>
            <span class="ascent-grade">{a.route_grade}</span>
            {#if a.route_name}<span class="ascent-name">{a.route_name}</span>{/if}

            {#if editingId === a.id}
              <form class="tick-edit" onsubmit={(e) => { e.preventDefault(); saveEdit(a.id); }}>
                <select bind:value={editForm.ascent_type}>
                  <option value="send">Send</option>
                  <option value="flash">Flash</option>
                  <option value="attempt">Attempt</option>
                  <option value="project">Project</option>
                </select>
                <input
                  type="number"
                  min="1"
                  step="1"
                  bind:value={editForm.attempts}
                  aria-label="Attempts" />
                <input
                  type="text"
                  bind:value={editForm.notes}
                  placeholder="Notes (optional)" />
                <div class="tick-edit-actions">
                  <button class="primary" type="submit" disabled={editSaving}>
                    {editSaving ? '…' : 'Save'}
                  </button>
                  <button type="button" disabled={editSaving} onclick={() => (editingId = null)}>
                    Cancel
                  </button>
                </div>
                {#if editError}<p class="error tick-error">{editError}</p>{/if}
              </form>
            {:else}
              <span class="type type-{a.ascent_type}">{a.ascent_type}</span>
              <span class="ascent-meta muted">
                {fmtDateTime(a.climbed_at)} · {a.attempts} attempt{a.attempts === 1 ? '' : 's'}
              </span>
              {#if a.notes}
                <span class="ascent-notes">{a.notes}</span>
              {/if}
              <span class="tick-actions">
                <button
                  type="button"
                  class="ghost"
                  disabled={mutatingId === a.id}
                  onclick={() => openEdit(a)}>
                  Edit
                </button>
                <button
                  type="button"
                  class="ghost danger"
                  disabled={mutatingId === a.id}
                  onclick={() => removeAscent(a.id)}>
                  {mutatingId === a.id ? '…' : 'Delete'}
                </button>
              </span>
            {/if}
          </li>
        {/each}
      </ul>

      {#if hasPrev || hasNext}
        <div class="pager">
          <button
            disabled={!hasPrev || loading}
            onclick={() => (offset = Math.max(0, offset - PAGE_SIZE))}>
            ← Prev
          </button>
          <button
            disabled={!hasNext || loading}
            onclick={() => (offset = offset + PAGE_SIZE)}>
            Next →
          </button>
        </div>
      {/if}
    {/if}
  </section>
</div>

<style>
  .page {
    max-width: 56rem;
  }
  .page-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  .who {
    display: flex;
    align-items: center;
    gap: 1rem;
  }
  .avatar {
    width: 64px;
    height: 64px;
    border-radius: 50%;
    object-fit: cover;
    border: 1px solid var(--rw-border-strong);
  }
  .avatar-fallback {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    font-weight: 700;
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
  .bio {
    margin: 0.4rem 0 0;
    color: var(--rw-text);
    max-width: 32rem;
  }
  .settings-link {
    color: var(--rw-text);
    text-decoration: none;
    font-weight: 600;
    font-size: 0.9rem;
    padding: 0.5rem 0.85rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
  }
  .settings-link:hover {
    border-color: var(--rw-accent);
    color: var(--rw-accent);
  }
  .stats-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(8rem, 1fr));
    gap: 0.75rem;
    margin-bottom: 1rem;
  }
  .stat {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1rem 1.1rem;
  }
  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    line-height: 1;
    margin-bottom: 0.25rem;
  }
  .stat-label {
    color: var(--rw-text-muted);
    font-size: 0.8rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .card h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.85rem;
  }
  .pyramid {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .pyr-row {
    display: grid;
    grid-template-columns: 4rem 1fr 3rem;
    align-items: center;
    gap: 8px;
    font-size: 0.85rem;
  }
  .pyr-grade {
    font-weight: 700;
    color: var(--rw-text);
  }
  .pyr-bar {
    background: var(--rw-surface-alt);
    border-radius: 4px;
    height: 14px;
    overflow: hidden;
  }
  .pyr-fill {
    display: block;
    background: var(--rw-accent);
    height: 100%;
  }
  .pyr-count {
    text-align: right;
    color: var(--rw-text-muted);
  }
  .results-meta {
    margin-bottom: 0.75rem;
    font-size: 0.85rem;
  }
  .ascent-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .ascent-list li {
    display: grid;
    grid-template-columns: 18px 3rem 1fr auto auto;
    grid-template-areas:
      'chip grade name type actions'
      'chip meta meta meta meta'
      'chip notes notes notes notes';
    column-gap: 10px;
    align-items: center;
    padding: 0.55rem 0;
    border-top: 1px solid var(--rw-border);
    font-size: 0.9rem;
  }
  .ascent-list li.editing {
    grid-template-columns: 18px 3rem 1fr;
    grid-template-areas:
      'chip grade name'
      'edit edit edit';
  }
  .tick-actions {
    grid-area: actions;
    display: inline-flex;
    gap: 4px;
  }
  .tick-actions button {
    background: transparent;
    border: 1px solid transparent;
    color: var(--rw-text-muted);
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 600;
    cursor: pointer;
  }
  .tick-actions button:hover:not(:disabled) {
    border-color: var(--rw-border-strong);
    color: var(--rw-text);
  }
  .tick-actions button.danger:hover:not(:disabled) {
    color: var(--rw-danger);
    border-color: #fecaca;
  }
  .tick-actions button:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .tick-edit {
    grid-area: edit;
    display: grid;
    grid-template-columns: 8rem 5rem 1fr;
    gap: 8px;
    align-items: center;
    margin-top: 6px;
  }
  .tick-edit select,
  .tick-edit input {
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.85rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .tick-edit-actions {
    grid-column: 1 / -1;
    display: inline-flex;
    gap: 6px;
  }
  .tick-edit-actions button {
    padding: 0.35rem 0.75rem;
    border-radius: 6px;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .tick-edit-actions button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
  }
  .tick-edit-actions button:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .tick-error {
    grid-column: 1 / -1;
    margin: 4px 0 0;
    font-size: 0.85rem;
  }
  .ascent-list li:first-child {
    border-top: none;
  }
  .color-chip {
    grid-area: chip;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .ascent-grade {
    grid-area: grade;
    font-weight: 700;
  }
  .ascent-name {
    grid-area: name;
    color: var(--rw-text);
  }
  .type {
    grid-area: type;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 1px 7px;
    border-radius: 4px;
    font-weight: 700;
    justify-self: end;
  }
  .type-send,
  .type-flash {
    background: rgba(22, 163, 74, 0.15);
    color: #15803d;
  }
  .type-attempt {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .ascent-meta {
    grid-area: meta;
    font-size: 0.8rem;
  }
  .ascent-notes {
    grid-area: notes;
    color: var(--rw-text);
    font-size: 0.88rem;
    margin-top: 2px;
  }
  .pager {
    display: flex;
    justify-content: space-between;
    margin-top: 0.75rem;
  }
  button {
    cursor: pointer;
    padding: 0.45rem 0.85rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.85rem;
    font-weight: 600;
  }
  button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
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
