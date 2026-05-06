<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    listEvents,
    type Competition,
    type CompetitionEvent,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let events = $state<CompetitionEvent[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const id = $derived(page.params.id ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/staff/comp/' + id + '/problems'));
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;
    try {
      const [c, evs] = await Promise.all([getCompetition(id), listEvents(id)]);
      comp = c;
      events = evs;
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load competition.';
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head>
  <title>Problems — {comp?.name ?? 'Competition'} — Routewerk</title>
</svelte:head>

<main>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <h1>Couldn't load competition</h1>
    <p class="error">{error}</p>
    <p><a href="/staff/comp">← Back to competitions</a></p>
  {:else if comp}
    <header>
      <a class="back" href="/staff/comp/{id}">← {comp.name}</a>
      <h1>Problems</h1>
      <p class="meta">Per-event problem editor — coming in Phase 1h.3.</p>
    </header>

    {#if events.length === 0}
      <p class="muted">No events yet. Add an event from the comp dashboard first.</p>
    {:else}
      <ul class="event-list">
        {#each events as ev (ev.id)}
          <li class="event">
            <div class="event-head">
              <span class="seq">#{ev.sequence}</span>
              <span class="name">{ev.name}</span>
            </div>
            <p class="placeholder muted">Problem list + create/edit form will land here.</p>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

<style>
  main {
    max-width: 48rem;
    margin: 0 auto;
    padding: 1.5rem 1rem 4rem;
  }
  .back {
    display: inline-block;
    color: #f97316;
    text-decoration: none;
    margin-bottom: 0.75rem;
    font-weight: 600;
  }
  h1 {
    font-size: 1.6rem;
    margin: 0 0 0.5rem;
  }
  .meta {
    color: #475569;
    margin: 0 0 1.5rem;
  }
  .event-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .event {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    padding: 1rem;
  }
  .event-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 0.5rem;
  }
  .seq {
    color: #94a3b8;
    font-weight: 600;
    font-size: 0.85rem;
  }
  .name {
    font-weight: 600;
    font-size: 1.05rem;
  }
  .placeholder {
    margin: 0;
    font-size: 0.9rem;
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
