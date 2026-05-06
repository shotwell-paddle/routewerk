<script lang="ts">
  import { page } from '$app/state';
  import {
    getQuest,
    listMyQuests,
    startQuest,
    logQuestProgress,
    abandonQuest,
    ApiClientError,
    type QuestShape,
    type ClimberQuestShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  let quest = $state<QuestShape | null>(null);
  let enrollment = $state<ClimberQuestShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let mutating = $state(false);
  let mutateError = $state<string | null>(null);

  let logForm = $state({ notes: '', log_type: 'general' });
  let logSaving = $state(false);

  const questId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());

  async function refresh() {
    if (!questId) return;
    const [q, mine] = await Promise.all([
      getQuest(questId),
      listMyQuests(),
    ]);
    quest = q;
    enrollment = mine.find((cq) => cq.quest_id === questId) ?? null;
  }

  $effect(() => {
    if (!questId) return;
    let cancelled = false;
    loading = true;
    error = null;
    refresh()
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load quest.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function start() {
    if (!quest || !locId) return;
    mutating = true;
    mutateError = null;
    try {
      await startQuest(quest.id, locId);
      await refresh();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not start quest.';
    } finally {
      mutating = false;
    }
  }

  async function abandon() {
    if (!enrollment) return;
    if (!confirm('Drop out of this quest? Your progress will be lost.')) return;
    mutating = true;
    mutateError = null;
    try {
      await abandonQuest(enrollment.id);
      await refresh();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not abandon quest.';
    } finally {
      mutating = false;
    }
  }

  async function logEntry(e: Event) {
    e.preventDefault();
    if (!enrollment) return;
    logSaving = true;
    mutateError = null;
    try {
      await logQuestProgress(enrollment.id, {
        log_type: logForm.log_type,
        notes: logForm.notes.trim() || null,
      });
      logForm = { notes: '', log_type: 'general' };
      await refresh();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not log progress.';
    } finally {
      logSaving = false;
    }
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString();
  }

  const progressPct = $derived.by(() => {
    if (!enrollment || !quest?.target_count) return 0;
    return Math.min(100, (enrollment.progress_count / quest.target_count) * 100);
  });
</script>

<svelte:head>
  <title>{quest?.name ?? 'Quest'} — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/quests">← Quests</a>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if quest}
    <header class="page-header">
      <div>
        {#if quest.domain}
          <span class="domain-chip" style="background:{quest.domain.color ?? '#cbd5e1'}">
            {quest.domain.icon ?? '◆'} {quest.domain.name}
          </span>
        {/if}
        <h1>{quest.name}</h1>
        <p class="meta-line muted">
          {quest.skill_level} ·
          {#if quest.target_count}target {quest.target_count}{/if}
          {#if quest.suggested_duration_days}· {quest.suggested_duration_days} days suggested{/if}
        </p>
      </div>
      {#if enrollment}
        <span class="status-pill status-{enrollment.status}">{enrollment.status}</span>
      {/if}
    </header>

    <section class="card">
      <h2>About</h2>
      <p class="prose">{quest.description}</p>
      {#if quest.completion_criteria}
        <p class="prose criteria">
          <strong>How to complete:</strong> {quest.completion_criteria}
        </p>
      {/if}
    </section>

    {#if !enrollment || enrollment.status === 'abandoned'}
      <section class="card cta-card">
        <h2>Get started</h2>
        <p class="muted">
          Start this quest to track your progress and earn the badge.
        </p>
        {#if mutateError}<p class="error">{mutateError}</p>{/if}
        <button class="primary" disabled={mutating || !locId} onclick={start}>
          {mutating ? 'Starting…' : 'Start quest'}
        </button>
      </section>
    {:else if enrollment.status === 'active'}
      <section class="card">
        <h2>Your progress</h2>
        {#if quest.target_count}
          <div class="progress-row">
            <div class="progress-bar">
              <span class="progress-fill" style="width: {progressPct}%"></span>
            </div>
            <span class="progress-text">
              {enrollment.progress_count} / {quest.target_count}
            </span>
          </div>
        {:else}
          <p class="muted">{enrollment.progress_count} entries logged</p>
        {/if}
        <p class="muted started-at">Started {fmtDate(enrollment.started_at)}</p>
      </section>

      <section class="card">
        <h2>Log progress</h2>
        <form onsubmit={logEntry}>
          <div class="field">
            <label for="lq-type">Type</label>
            <select id="lq-type" bind:value={logForm.log_type}>
              <option value="general">General</option>
              <option value="ascent">Ascent</option>
              <option value="practice">Practice</option>
              <option value="milestone">Milestone</option>
            </select>
          </div>
          <div class="field">
            <label for="lq-notes">Notes</label>
            <textarea
              id="lq-notes"
              bind:value={logForm.notes}
              rows="3"
              placeholder="What did you do?"></textarea>
          </div>
          {#if mutateError}<p class="error">{mutateError}</p>{/if}
          <button class="primary" type="submit" disabled={logSaving}>
            {logSaving ? 'Logging…' : 'Log entry'}
          </button>
        </form>
      </section>

      <section class="card danger-zone">
        <h2>Abandon</h2>
        <p class="muted">
          Dropping out clears your progress on this quest. You can start it
          again later.
        </p>
        <button class="danger" disabled={mutating} onclick={abandon}>
          {mutating ? '…' : 'Drop quest'}
        </button>
      </section>
    {:else if enrollment.status === 'completed'}
      <section class="card success-card">
        <h2>Completed</h2>
        <p>
          You completed this quest{enrollment.completed_at
            ? ` on ${fmtDate(enrollment.completed_at)}`
            : ''}. Nice work.
        </p>
      </section>
    {/if}
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
  .page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  .domain-chip {
    display: inline-block;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    color: #fff;
    font-weight: 700;
    text-shadow: 0 1px 0 rgba(0, 0, 0, 0.15);
    margin-bottom: 8px;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
    letter-spacing: -0.01em;
  }
  h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.75rem;
  }
  .meta-line {
    margin: 0;
    font-size: 0.9rem;
  }
  .status-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .status-active {
    background: rgba(245, 158, 11, 0.2);
    color: #92590a;
  }
  .status-completed {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-abandoned {
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
  .prose {
    color: var(--rw-text);
    line-height: 1.5;
    margin: 0 0 0.5rem;
    white-space: pre-wrap;
  }
  .criteria {
    color: var(--rw-text-muted);
    margin: 0;
  }
  .cta-card {
    border-color: var(--rw-accent);
  }
  .progress-row {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .progress-bar {
    flex: 1;
    background: var(--rw-surface-alt);
    border-radius: 4px;
    height: 12px;
    overflow: hidden;
  }
  .progress-fill {
    display: block;
    background: var(--rw-accent);
    height: 100%;
  }
  .progress-text {
    font-weight: 600;
    color: var(--rw-text);
  }
  .started-at {
    margin: 0.5rem 0 0;
    font-size: 0.85rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 0.75rem;
  }
  .field label {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--rw-text-muted);
  }
  .field select,
  .field textarea {
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    box-sizing: border-box;
    font-family: inherit;
  }
  button {
    cursor: pointer;
    padding: 0.55rem 1rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.9rem;
    font-weight: 600;
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
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
  }
  button.danger:hover:not(:disabled) {
    background: #fef2f2;
  }
  .danger-zone {
    border-color: #fde2e2;
    background: #fffafa;
  }
  .danger-zone h2 {
    color: #991b1b;
  }
  .success-card {
    background: rgba(22, 163, 74, 0.06);
    border-color: rgba(22, 163, 74, 0.25);
  }
  .success-card h2 {
    color: #15803d;
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
