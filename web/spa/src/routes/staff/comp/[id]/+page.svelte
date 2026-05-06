<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { getCompetition, type Competition, ApiClientError } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const id = $derived(page.params.id ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/staff/comp/' + id));
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;
    try {
      comp = await getCompetition(id);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load competition.';
    } finally {
      loading = false;
    }
  });

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString();
  }
</script>

<svelte:head>
  <title>{comp?.name ?? 'Competition'} — Routewerk staff</title>
</svelte:head>

<main>
  <p><a href="/staff/comp">← Back to competitions</a></p>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if comp}
    <header>
      <h1>{comp.name}</h1>
      <p class="meta">
        <span class="status" data-status={comp.status}>{comp.status}</span>
        · {fmtDate(comp.starts_at)} → {fmtDate(comp.ends_at)}
        · scoring: <code>{comp.scoring_rule}</code>
      </p>
      <p class="meta muted">Slug: <code>{comp.slug}</code></p>
    </header>

    <section class="placeholder">
      <h2>Dashboard coming in 1h.2</h2>
      <p>
        This is the foundation page for the staff comp dashboard. The
        next sub-PR adds: edit comp metadata, manage events
        + categories inline, and a link out to the problems editor.
        After that, 1h.3 (problems editor) and 1h.4 (registrations
        + verify queue) round out the staff UI.
      </p>
      <p>
        For now you can preview the comp at
        <a href={`/comp/${comp.id}`}>climber view →</a>
      </p>
    </section>
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
    margin: 0 0 0.5rem;
    color: #c2410c;
  }
  .meta {
    color: #475569;
    margin: 0.25rem 0;
  }
  .status {
    text-transform: capitalize;
    font-size: 0.85rem;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 999px;
    background: #f1f5f9;
    color: #475569;
  }
  .status[data-status='draft'] {
    background: #fef3c7;
    color: #92400e;
  }
  .status[data-status='live'] {
    background: #ecfdf5;
    color: #047857;
  }
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
  }
  .placeholder {
    margin-top: 1.5rem;
    background: #fff7ed;
    border: 1px solid #fed7aa;
    border-radius: 8px;
    padding: 1.25rem;
  }
  .placeholder p {
    color: #78350f;
    margin: 0 0 0.5rem;
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
