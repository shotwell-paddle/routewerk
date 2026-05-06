<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    listCompetitions,
    getLocation,
    type Competition,
    type LocationShape,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated, currentUser } from '$lib/stores/auth.svelte';

  // A "staff location" = a location the user can manage comps at.
  // Server enforces head_setter+ for create; we filter the same way
  // client-side so the UI only shows locations they can act on.
  const STAFF_ROLES = new Set(['head_setter', 'gym_manager', 'org_admin']);

  interface Section {
    location: LocationShape;
    role: string;
    comps: Competition[];
  }

  let loading = $state(true);
  let error = $state<string | null>(null);
  let sections = $state<Section[]>([]);

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/staff/comp'));
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;

    try {
      const a = authState();
      const me = a.me;
      if (!me) return;

      // Pick memberships at head_setter+ that have a location_id.
      const staffLocations = me.memberships
        .filter((m) => STAFF_ROLES.has(m.role) && m.location_id)
        .map((m) => ({ location_id: m.location_id as string, role: m.role }));

      // Load location metadata + comps in parallel per location.
      const built = await Promise.all(
        staffLocations.map(async (m) => {
          const [loc, comps] = await Promise.all([
            getLocation(m.location_id),
            listCompetitions(m.location_id).catch(() => []),
          ]);
          if (!loc) return null;
          return { location: loc, role: m.role, comps } satisfies Section;
        }),
      );
      sections = built.filter((s): s is Section => s !== null);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load competitions.';
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
  <title>Competitions — Routewerk staff</title>
</svelte:head>

<main>
  <header>
    <h1>Competitions</h1>
    {#if currentUser()}
      <p class="muted">Signed in as {currentUser()?.display_name}</p>
    {/if}
  </header>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if sections.length === 0}
    <div class="empty">
      <h2>No staff access</h2>
      <p class="muted">
        You don't have <code>head_setter</code> (or higher) at any location, so
        you can't create or manage competitions here.
      </p>
      <p>
        <a href="/dashboard">← Back to dashboard</a>
      </p>
    </div>
  {:else}
    <p class="actions">
      <a class="btn" href="/staff/comp/new">+ New competition</a>
    </p>
    {#each sections as section (section.location.id)}
      <section class="loc">
        <h2>{section.location.name}</h2>
        {#if section.comps.length === 0}
          <p class="muted">No competitions yet.</p>
        {:else}
          <ul class="comps">
            {#each section.comps as comp (comp.id)}
              <li>
                <a class="comp-row" href={`/staff/comp/${comp.id}`}>
                  <div>
                    <div class="comp-name">{comp.name}</div>
                    <div class="comp-meta muted">
                      {fmtDate(comp.starts_at)} – {fmtDate(comp.ends_at)} · scoring: <code>{comp.scoring_rule}</code>
                    </div>
                  </div>
                  <span class="status" data-status={comp.status}>{comp.status}</span>
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/each}
  {/if}
</main>

<style>
  main {
    max-width: 48rem;
    margin: 0 auto;
    padding: 1.5rem 1rem 4rem;
  }
  h1 {
    font-size: 1.6rem;
    margin: 0 0 0.5rem;
  }
  h2 {
    font-size: 1.1rem;
    margin: 0 0 0.75rem;
    color: #475569;
  }
  .muted {
    color: #94a3b8;
  }
  .actions {
    margin: 0 0 1.5rem;
  }
  .btn {
    display: inline-block;
    padding: 0.6rem 1rem;
    background: #f97316;
    color: #fff;
    border-radius: 8px;
    text-decoration: none;
    font-weight: 600;
  }
  .loc {
    margin-bottom: 2rem;
  }
  .comps {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .comp-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.85rem 1rem;
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    text-decoration: none;
    color: inherit;
  }
  .comp-row:hover {
    border-color: #f97316;
  }
  .comp-name {
    font-weight: 600;
  }
  .comp-meta {
    font-size: 0.85rem;
    margin-top: 2px;
  }
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
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
  .status[data-status='archived'] {
    background: #f5f5f5;
    color: #94a3b8;
  }
  .empty {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 12px;
    padding: 2rem;
    text-align: center;
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
