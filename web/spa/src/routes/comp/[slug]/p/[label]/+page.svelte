<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    getCompetitionBySlug,
    listEvents,
    listProblems,
    listRegistrations,
    listRegistrationAttempts,
    type Competition,
    type CompetitionProblem,
    type CompetitionRegistration,
    ApiClientError,
  } from '$lib/api/client';
  import { ActionQueue } from '$lib/stores/actions.svelte';
  import { authState, currentUser, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let problem = $state<CompetitionProblem | null>(null);
  let registration = $state<CompetitionRegistration | null>(null);
  let queue = $state<ActionQueue | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const slugOrId = $derived(page.params.slug ?? '');
  const label = $derived(page.params.label ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto(
        `/sign-in?next=${encodeURIComponent('/comp/' + slugOrId + '/p/' + label)}`,
      );
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;
    try {
      await loadAll();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load this problem.';
    } finally {
      loading = false;
    }
  });

  function isUUID(s: string): boolean {
    return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s);
  }

  async function loadAll() {
    comp = isUUID(slugOrId)
      ? await getCompetition(slugOrId)
      : await getCompetitionBySlug(slugOrId);

    // Find the problem by label across the comp's events.
    const events = await listEvents(comp.id);
    const decoded = decodeURIComponent(label);
    for (const ev of events) {
      const ps = await listProblems(ev.id);
      const match = ps.find((p) => p.label === decoded);
      if (match) {
        problem = match;
        break;
      }
    }
    if (!problem) {
      error = `No problem labeled "${decoded}" in this competition.`;
      return;
    }

    const me = currentUser();
    if (!me) return;
    const regs = await listRegistrations(comp.id);
    const mine = regs.find((r) => r.user_id === me.id && !r.withdrawn_at);
    if (!mine) {
      error = 'You need to register for this comp first.';
      return;
    }
    registration = mine;
    const attempts = await listRegistrationAttempts(mine.id);
    queue = new ActionQueue(comp.id);
    queue.hydrate(attempts);
  }

  // Reactive references to the queue's per-problem state for THIS problem.
  // Named `attemptState` so we don't shadow Svelte's `$state` rune.
  const attemptState = $derived(queue && problem ? queue.attempts[problem.id] : undefined);
  const errMsg = $derived(queue && problem ? queue.errors[problem.id] : null);
  const pending = $derived(!!(queue && problem && queue.isPending(problem.id)));

  async function tap(type: 'increment' | 'zone' | 'top' | 'undo' | 'reset') {
    if (!queue || !problem) return;
    await queue.submit(problem.id, type);
  }
</script>

<svelte:head>
  <title>{problem ? `${problem.label} · ${comp?.name ?? ''}` : 'Problem'} — Routewerk</title>
</svelte:head>

<main>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
    <p><a href="/comp/{slugOrId}">← Back to scorecard</a></p>
  {:else if problem && queue}
    <header>
      <a class="back" href="/comp/{slugOrId}">← Scorecard</a>
      <h1>{problem.label}</h1>
      {#if problem.color || problem.grade}
        <p class="meta">
          {#if problem.color}<span class="color-dot" style:background={problem.color}></span>{/if}
          {#if problem.grade}<span>{problem.grade}</span>{/if}
        </p>
      {/if}
    </header>

    <div class="state-summary" data-status={attemptState?.top_reached ? 'top' : attemptState?.zone_reached ? 'zone' : attemptState && attemptState.attempts > 0 ? 'tries' : 'idle'}>
      <div class="big-number">{attemptState?.attempts ?? 0}</div>
      <div class="big-label">attempts</div>
      <div class="badges">
        <span class="badge" class:active={attemptState?.zone_reached}>
          zone{attemptState?.zone_reached && attemptState.zone_attempts != null ? ` · ${attemptState.zone_attempts}` : ''}
        </span>
        <span class="badge" class:active={attemptState?.top_reached}>
          top
        </span>
      </div>
    </div>

    {#if errMsg}
      <p class="error">{errMsg}</p>
    {/if}

    <div class="actions">
      <button
        type="button"
        class="primary"
        onclick={() => tap('increment')}
        disabled={pending}
      >
        +1 Attempt
      </button>
      <button
        type="button"
        class="zone"
        onclick={() => tap('zone')}
        disabled={pending}
      >
        Got Zone
      </button>
      <button
        type="button"
        class="top"
        onclick={() => tap('top')}
        disabled={pending}
      >
        Sent It
      </button>
    </div>

    <div class="secondary-actions">
      <button type="button" class="undo" onclick={() => tap('undo')} disabled={pending}>
        Undo
      </button>
      <button type="button" class="reset" onclick={() => tap('reset')} disabled={pending}>
        Reset
      </button>
    </div>
  {/if}
</main>

<style>
  main {
    max-width: 32rem;
    margin: 0 auto;
    padding: 1rem 1rem 4rem;
  }
  .back {
    display: inline-block;
    color: #f97316;
    text-decoration: none;
    margin-bottom: 0.75rem;
    font-weight: 600;
  }
  h1 {
    font-size: 2rem;
    margin: 0 0 0.5rem;
  }
  .meta {
    color: #64748b;
    margin: 0 0 1rem;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .color-dot {
    display: inline-block;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1px solid rgba(0, 0, 0, 0.15);
  }
  .state-summary {
    background: #f1f5f9;
    border: 1px solid #e2e8f0;
    border-radius: 12px;
    padding: 1.5rem;
    text-align: center;
    margin: 1rem 0 1.5rem;
  }
  .state-summary[data-status='top'] {
    background: #ecfdf5;
    border-color: #a7f3d0;
  }
  .state-summary[data-status='zone'] {
    background: #fef3c7;
    border-color: #fde68a;
  }
  .big-number {
    font-size: 3rem;
    font-weight: 700;
    line-height: 1;
    color: #0f172a;
  }
  .big-label {
    color: #64748b;
    margin-top: 4px;
  }
  .badges {
    display: flex;
    gap: 8px;
    justify-content: center;
    margin-top: 12px;
  }
  .badge {
    padding: 4px 12px;
    border-radius: 999px;
    background: #fff;
    border: 1px solid #cbd5e1;
    color: #94a3b8;
    font-size: 0.85rem;
    font-weight: 600;
  }
  .badge.active {
    background: #f97316;
    border-color: #f97316;
    color: #fff;
  }
  .actions {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .actions button {
    padding: 1.5rem;
    font-size: 1.4rem;
    font-weight: 700;
    border: 0;
    border-radius: 12px;
    color: #fff;
    cursor: pointer;
    touch-action: manipulation;
  }
  .actions button:active {
    transform: scale(0.98);
  }
  .actions button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .actions .primary {
    background: #475569;
  }
  .actions .zone {
    background: #d97706;
  }
  .actions .top {
    background: #059669;
  }
  .secondary-actions {
    display: flex;
    gap: 8px;
    margin-top: 1.5rem;
  }
  .secondary-actions button {
    flex: 1;
    padding: 0.75rem;
    background: #fff;
    border: 1px solid #cbd5e1;
    border-radius: 8px;
    font-weight: 600;
    color: #64748b;
    cursor: pointer;
  }
  .secondary-actions button:disabled {
    opacity: 0.5;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
  .muted {
    color: #94a3b8;
  }
</style>
