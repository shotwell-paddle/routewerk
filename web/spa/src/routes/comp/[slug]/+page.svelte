<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    type Competition,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Phase 1g.1 scaffold: the [slug] route param is treated as a UUID
  // until 1g.2 introduces a slug→id resolver endpoint that searches
  // the user's accessible locations. For the foundation PR we just
  // prove auth + routing + typed API client work end-to-end.
  // SvelteKit types params as `Record<string, string | undefined>`
  // because the file's [slug] segment is a moving target — narrow
  // explicitly. The route file's path guarantees this is set.
  const id = $derived(page.params.slug ?? '');

  // Bounce unauthenticated users to /login with a `next` so they come
  // back to this same comp after sign-in.
  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto(`/login?next=${encodeURIComponent('/comp/' + id)}`);
    }
  });

  onMount(async () => {
    try {
      comp = await getCompetition(id);
    } catch (err) {
      if (err instanceof ApiClientError) {
        error =
          err.status === 404
            ? 'This competition was not found.'
            : err.status === 401
              ? 'Please sign in to view this competition.'
              : (typeof err.body === 'object' ? err.body.error : err.message);
      } else {
        error = 'Could not load the competition.';
      }
    } finally {
      loading = false;
    }
  });

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
  {#if loading && !isAuthenticated()}
    <p class="muted">Checking sign-in…</p>
  {:else if loading}
    <p class="muted">Loading competition…</p>
  {:else if error}
    <h1>Couldn't load this comp</h1>
    <p class="error">{error}</p>
    <p><a href="/comp">Back to competitions</a></p>
  {:else if comp}
    <header>
      <h1>{comp.name}</h1>
      <p class="meta">
        {fmtDate(comp.starts_at)} – {fmtDate(comp.ends_at)} ·
        <span class="status">{comp.status}</span> · scoring: <code>{comp.scoring_rule}</code>
      </p>
    </header>
    <section class="placeholder">
      <h2>Scorecard coming in 1g.2</h2>
      <p>
        This page is the Phase 1g.1 foundation — auth, routing, and the typed
        API client are wired up. The next sub-PR adds the actual scorecard:
        problem cards, the three big tap targets, optimistic UI, and an action
        queue that retries on flaky network.
      </p>
    </section>
  {/if}
</main>

<style>
  main {
    max-width: 40rem;
    margin: 0 auto;
    padding: 2rem 1rem;
  }
  h1 {
    font-size: 1.75rem;
    margin: 0 0 0.5rem;
  }
  .meta {
    color: #475569;
    margin: 0 0 1.5rem;
  }
  .status {
    text-transform: capitalize;
  }
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
  }
  .placeholder {
    background: #fff7ed;
    border: 1px solid #fed7aa;
    border-radius: 8px;
    padding: 1.25rem;
  }
  .placeholder h2 {
    margin: 0 0 0.5rem;
    font-size: 1.1rem;
    color: #c2410c;
  }
  .placeholder p {
    margin: 0;
    color: #78350f;
    font-size: 0.95rem;
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
