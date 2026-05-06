<script lang="ts">
  import { page } from '$app/state';
  import {
    getSession,
    updateSession,
    listWalls,
    ApiClientError,
    type SessionShape,
    type SessionStatus,
    type SessionWriteShape,
    type WallShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import SessionForm from '$lib/components/SessionForm.svelte';

  let session = $state<SessionShape | null>(null);
  let walls = $state<WallShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let editing = $state(false);
  let saving = $state(false);
  let saveError = $state<string | null>(null);

  const sessionId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());

  $effect(() => {
    if (!locId || !sessionId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getSession(locId, sessionId),
      // Walls feed assignment-card wall labels.
      listWalls(locId).catch(() => [] as WallShape[]),
    ])
      .then(([s, w]) => {
        if (cancelled) return;
        session = s;
        walls = w;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load session.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function submitEdit(body: SessionWriteShape) {
    if (!locId || !sessionId) return;
    saving = true;
    saveError = null;
    try {
      session = await updateSession(locId, sessionId, body);
      editing = false;
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      saving = false;
    }
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
      year: 'numeric',
    });
  }

  function wallName(id: string | null | undefined): string {
    if (!id) return 'Any wall';
    return walls.find((w) => w.id === id)?.name ?? '—';
  }

  const STATUS_LABEL: Record<SessionStatus, string> = {
    planning: 'Planning',
    in_progress: 'In progress',
    complete: 'Complete',
    cancelled: 'Cancelled',
  };
</script>

<svelte:head>
  <title>Session {session ? fmtDate(session.scheduled_date) : ''} — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/sessions">← Sessions</a>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if session}
    <header class="page-header">
      <div>
        <h1>{fmtDate(session.scheduled_date)}</h1>
        <p class="meta-line">
          <span class="status-pill status-{session.status}">
            {STATUS_LABEL[session.status as SessionStatus] ?? session.status}
          </span>
        </p>
      </div>
      {#if !editing}
        <button onclick={() => (editing = true)}>Edit</button>
      {/if}
    </header>

    {#if editing}
      <SessionForm
        initial={session}
        submitLabel="Save changes"
        onSubmit={submitEdit}
        onCancel={() => {
          editing = false;
          saveError = null;
        }}
        {saving}
        error={saveError} />
    {:else}
      {#if session.notes}
        <section class="card">
          <h2>Notes</h2>
          <p class="prose">{session.notes}</p>
        </section>
      {/if}

      <section class="card">
        <h2>Assignments</h2>
        {#if !session.assignments || session.assignments.length === 0}
          <p class="muted">
            No setters assigned yet. Assignment management is coming in a
            follow-up — use
            <a class="link" href="/sessions/{session.id}">the existing session view</a>
            to assign setters in the meantime.
          </p>
        {:else}
          <ul class="assignment-list">
            {#each session.assignments as a (a.id)}
              <li>
                <span class="setter">Setter {a.setter_id.slice(0, 8)}…</span>
                <span class="muted">
                  · {wallName(a.wall_id)}
                  {#if a.target_grades && a.target_grades.length > 0}
                    · grades: {a.target_grades.join(', ')}
                  {/if}
                </span>
                {#if a.notes}<p class="assn-notes">{a.notes}</p>{/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="card">
        <h2>Lifecycle</h2>
        <p class="muted">
          Session status transitions, strip-target tracking, and the
          completion checklist live on
          <a class="link" href="/sessions/{session.id}">the existing session view</a>
          for now.
        </p>
      </section>
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
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
  .page-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }
  h1 {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0;
    letter-spacing: -0.01em;
  }
  .meta-line {
    margin: 6px 0 0;
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
    margin: 0 0 0.75rem;
  }
  .prose {
    margin: 0;
    color: var(--rw-text);
    line-height: 1.5;
    white-space: pre-wrap;
  }
  .assignment-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .assignment-list li {
    border-top: 1px solid var(--rw-border);
    padding-top: 8px;
  }
  .assignment-list li:first-child {
    border-top: none;
    padding-top: 0;
  }
  .setter {
    font-weight: 600;
  }
  .assn-notes {
    margin: 4px 0 0;
    font-size: 0.9rem;
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
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
</style>
