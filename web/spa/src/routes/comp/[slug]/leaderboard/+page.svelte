<script lang="ts">
  import { page } from '$app/state';
  import { onMount, onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    getCompetitionBySlug,
    type Competition,
    type LeaderboardEntry,
    ApiClientError,
  } from '$lib/api/client';
  import { LeaderboardStore } from '$lib/stores/leaderboard.svelte';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let store = $state<LeaderboardStore | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const slugOrId = $derived(page.params.slug ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto(
        `/sign-in?next=${encodeURIComponent('/comp/' + slugOrId + '/leaderboard')}`,
      );
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;
    try {
      comp = isUUID(slugOrId)
        ? await getCompetition(slugOrId)
        : await getCompetitionBySlug(slugOrId);
      store = new LeaderboardStore(comp.id);
      store.connect();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load leaderboard.';
    } finally {
      loading = false;
    }
  });

  onDestroy(() => {
    store?.close();
  });

  function isUUID(s: string): boolean {
    return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s);
  }

  // Format the headline metric per scorer. top_zone shows tops/zones;
  // points-based scorers show points.
  function metric(entry: LeaderboardEntry, rule: string | undefined): string {
    if (rule === 'top_zone') {
      return `${entry.tops}T ${entry.zones}Z`;
    }
    return entry.points.toFixed(0);
  }

  // Secondary line — the "tiebreak" data that explains the rank.
  function secondary(entry: LeaderboardEntry, rule: string | undefined): string {
    if (rule === 'top_zone') {
      return `${entry.attempts_to_top} att-to-top · ${entry.attempts_to_zone} att-to-zone`;
    }
    return `${entry.tops}T ${entry.zones}Z · ${entry.attempts_to_top} att`;
  }
</script>

<svelte:head>
  <title>{comp?.name ?? 'Leaderboard'} — Routewerk</title>
</svelte:head>

<main>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <h1>Couldn't load leaderboard</h1>
    <p class="error">{error}</p>
    <p><a href="/comp/{slugOrId}">← Back to scorecard</a></p>
  {:else if comp && store}
    <header>
      <a class="back" href="/comp/{slugOrId}">← Scorecard</a>
      <h1>
        Leaderboard
        <span class="live-dot" class:on={store.connected} title={store.connected ? 'Live' : 'Reconnecting…'}></span>
      </h1>
      <p class="meta">{comp.name} · scoring: <code>{comp.scoring_rule}</code></p>
    </header>

    {#if !store.board}
      <p class="muted">Waiting for first frame…</p>
    {:else if store.board.entries.length === 0}
      <p class="muted">No registered climbers have logged anything yet.</p>
    {:else}
      <ol class="board">
        {#each store.board.entries as entry (entry.registration_id)}
          <li class="row" data-rank-band={entry.rank <= 3 ? entry.rank : 'rest'}>
            <span class="rank">{entry.rank}</span>
            <span class="name">
              {entry.display_name}
              {#if entry.bib_number != null}
                <span class="bib">#{entry.bib_number}</span>
              {/if}
            </span>
            <span class="metric">{metric(entry, comp.scoring_rule)}</span>
            <span class="secondary">{secondary(entry, comp.scoring_rule)}</span>
          </li>
        {/each}
      </ol>
      <p class="generated muted">
        Updated {new Date(store.board.generated_at).toLocaleTimeString()}
      </p>
    {/if}
  {/if}
</main>

<style>
  main {
    max-width: 40rem;
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
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .live-dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: #94a3b8;
    transition: background 0.2s;
  }
  .live-dot.on {
    background: #16a34a;
    box-shadow: 0 0 0 4px rgba(22, 163, 74, 0.18);
  }
  .meta {
    color: #475569;
    margin: 0 0 1.5rem;
  }
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
  }
  .board {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .row {
    display: grid;
    grid-template-columns: 2.5rem 1fr auto;
    grid-template-areas:
      'rank name metric'
      'rank secondary secondary';
    align-items: center;
    gap: 4px 12px;
    padding: 0.85rem 1rem;
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
  }
  .row[data-rank-band='1'] {
    background: #fffbeb;
    border-color: #fde68a;
  }
  .row[data-rank-band='2'] {
    background: #f8fafc;
    border-color: #cbd5e1;
  }
  .row[data-rank-band='3'] {
    background: #fef3e8;
    border-color: #fed7aa;
  }
  .rank {
    grid-area: rank;
    font-weight: 700;
    font-size: 1.5rem;
    color: #475569;
    text-align: center;
  }
  .row[data-rank-band='1'] .rank {
    color: #b45309;
  }
  .name {
    grid-area: name;
    font-weight: 600;
    font-size: 1.05rem;
  }
  .bib {
    color: #94a3b8;
    font-weight: 400;
    font-size: 0.85rem;
    margin-left: 6px;
  }
  .metric {
    grid-area: metric;
    font-weight: 700;
    font-size: 1.2rem;
    color: #0f172a;
  }
  .secondary {
    grid-area: secondary;
    color: #64748b;
    font-size: 0.85rem;
  }
  .generated {
    margin-top: 1rem;
    text-align: right;
    font-size: 0.85rem;
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
  a {
    color: #f97316;
  }
</style>
