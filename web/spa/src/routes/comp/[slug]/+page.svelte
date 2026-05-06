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
    createRegistration,
    listCategories,
    listRegistrationAttempts,
    type Competition,
    type CompetitionEvent,
    type CompetitionProblem,
    type CompetitionRegistration,
    ApiClientError,
  } from '$lib/api/client';
  import { ActionQueue } from '$lib/stores/actions.svelte';
  import { authState, currentUser, isAuthenticated } from '$lib/stores/auth.svelte';
  import ProblemCard from '$lib/components/ProblemCard.svelte';

  let comp = $state<Competition | null>(null);
  let events = $state<CompetitionEvent[]>([]);
  let problemsByEvent = $state<Record<string, CompetitionProblem[]>>({});
  let registration = $state<CompetitionRegistration | null>(null);
  let queue = $state<ActionQueue | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let registering = $state(false);

  const slugOrId = $derived(page.params.slug ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto(`/login?next=${encodeURIComponent('/comp/' + slugOrId)}`);
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
      error = err instanceof ApiClientError ? err.message : 'Could not load the competition.';
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

    events = await listEvents(comp.id);
    const problemArrays = await Promise.all(events.map((e) => listProblems(e.id)));
    problemsByEvent = {};
    for (let i = 0; i < events.length; i++) {
      problemsByEvent[events[i].id] = problemArrays[i];
    }

    const me = currentUser();
    if (!me) return;
    const regs = await listRegistrations(comp.id);
    const mine = regs.find((r) => r.user_id === me.id && !r.withdrawn_at);
    if (mine) {
      registration = mine;
      const attempts = await listRegistrationAttempts(mine.id);
      queue = new ActionQueue(comp.id);
      queue.hydrate(attempts);
    }
  }

  // ── Self-registration prompt (no category picker for v1; users
  //     register against the first available category. Multi-category
  //     UX is a 1g polish item.) ─────────────────────────────────────

  async function register() {
    if (!comp || registering) return;
    registering = true;
    try {
      const cats = await listCategories(comp.id);
      if (cats.length === 0) {
        error = 'This competition has no categories yet — staff need to set one up.';
        return;
      }
      registration = await createRegistration(comp.id, {
        category_id: cats[0].id,
      });
      queue = new ActionQueue(comp.id);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Registration failed.';
    } finally {
      registering = false;
    }
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  }
</script>

<svelte:head>
  <title>{comp?.name ?? 'Competition'} — Routewerk</title>
</svelte:head>

<main>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <h1>Couldn't load this comp</h1>
    <p class="error">{error}</p>
  {:else if comp}
    <header>
      <h1>{comp.name}</h1>
      <p class="meta">
        {fmtDate(comp.starts_at)} – {fmtDate(comp.ends_at)} ·
        <span class="status">{comp.status}</span>
      </p>
      <p class="leaderboard-link">
        <a href="/comp/{slugOrId}/leaderboard">View leaderboard →</a>
      </p>
    </header>

    {#if !registration}
      <section class="register-card">
        <h2>Join this competition</h2>
        <p class="muted">Tap to register and start scoring.</p>
        <button type="button" onclick={register} disabled={registering}>
          {registering ? 'Registering…' : 'Register'}
        </button>
      </section>
    {:else if queue}
      {#each events as event (event.id)}
        <section class="event">
          <h2>{event.name}</h2>
          {#if (problemsByEvent[event.id] ?? []).length === 0}
            <p class="muted">No problems set yet.</p>
          {:else}
            <div class="problems">
              {#each problemsByEvent[event.id] as problem (problem.id)}
                <ProblemCard
                  {problem}
                  state={queue.attempts[problem.id]}
                  href={`/comp/${slugOrId}/p/${encodeURIComponent(problem.label)}`}
                />
              {/each}
            </div>
          {/if}
        </section>
      {/each}
    {/if}
  {/if}
</main>

<style>
  main {
    max-width: 40rem;
    margin: 0 auto;
    padding: 1.5rem 1rem 4rem;
  }
  h1 {
    font-size: 1.6rem;
    margin: 0 0 0.5rem;
  }
  h2 {
    font-size: 1.1rem;
    margin: 1.5rem 0 0.75rem;
    color: #475569;
  }
  .meta {
    color: #475569;
    margin: 0 0 0.5rem;
  }
  .status {
    text-transform: capitalize;
  }
  .leaderboard-link a {
    color: #f97316;
    text-decoration: none;
    font-weight: 600;
  }
  .leaderboard-link a:hover {
    text-decoration: underline;
  }
  .event h2:first-child {
    margin-top: 0;
  }
  .problems {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .register-card {
    margin-top: 2rem;
    padding: 1.5rem;
    background: #fff7ed;
    border: 1px solid #fed7aa;
    border-radius: 10px;
  }
  .register-card h2 {
    margin: 0 0 0.5rem;
    color: #9a3412;
  }
  .register-card button {
    width: 100%;
    padding: 0.7rem;
    background: #f97316;
    color: #fff;
    border: 0;
    border-radius: 8px;
    font-weight: 600;
    cursor: pointer;
    margin-top: 0.5rem;
  }
  .register-card button:disabled {
    background: #fbbf24;
  }
  .muted {
    color: #94a3b8;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
</style>
