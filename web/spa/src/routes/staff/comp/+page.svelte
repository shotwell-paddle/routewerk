<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    listCompetitions,
    listOrgLocations,
    type Competition,
    type LocationShape,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated, currentUser } from '$lib/stores/auth.svelte';
  import { effectiveRoleAt, roleRankAt } from '$lib/stores/auth.svelte';

  // A "staff location" = a location the user can manage comps at, i.e. their
  // effective role there is head_setter+ (rank 3+). We resolve the effective
  // role via the shared helper so org-wide memberships (location_id null)
  // and is_app_admin both promote correctly — without that, an org_admin
  // whose row has no location_id sees "you don't have head_setter anywhere".

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

      // Build the candidate location set. Location-scoped memberships
      // contribute their own location_id; org-wide memberships need an
      // extra fetch to enumerate the org's locations (so an org_admin
      // sees every gym in their org, not just an empty list).
      const locationIds = new Set<string>();
      const orgIdsForOrgWide = new Set<string>();
      for (const m of me.memberships) {
        if (m.location_id) {
          locationIds.add(m.location_id);
        } else {
          orgIdsForOrgWide.add(m.org_id);
        }
      }
      // is_app_admin: list every org the user touches via memberships and
      // pull every location. We don't have a global "list all orgs"
      // endpoint, so this only sees orgs the user has any membership in
      // — which is correct for a "manage *your* gyms" UX even at app-admin
      // level.
      if (me.user.is_app_admin) {
        for (const m of me.memberships) orgIdsForOrgWide.add(m.org_id);
      }
      const orgLocLists = await Promise.all(
        Array.from(orgIdsForOrgWide).map((orgId) =>
          listOrgLocations(orgId).catch(() => [] as LocationShape[]),
        ),
      );
      for (const list of orgLocLists) {
        for (const loc of list) locationIds.add(loc.id);
      }

      // Filter to locations where the caller is head_setter+ (rank 3+).
      const staffIds = Array.from(locationIds).filter((id) => roleRankAt(id) >= 3);

      // Load location metadata + comps in parallel per location.
      const built = await Promise.all(
        staffIds.map(async (id) => {
          const role = effectiveRoleAt(id) ?? '';
          // Reuse the org list responses where possible; if the location
          // came from a location-scoped membership we don't have its row
          // cached, so resolve via getLocation.
          let loc: LocationShape | null = null;
          for (const list of orgLocLists) {
            const found = list.find((l) => l.id === id);
            if (found) {
              loc = found;
              break;
            }
          }
          if (!loc) {
            const { getLocation } = await import('$lib/api/client');
            loc = await getLocation(id);
          }
          if (!loc) return null;
          const comps = await listCompetitions(id).catch(() => []);
          return { location: loc, role, comps } satisfies Section;
        }),
      );
      sections = built
        .filter((s): s is Section => s !== null)
        .sort((a, b) => a.location.name.localeCompare(b.location.name));
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
    background: var(--rw-accent);
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
    border-color: var(--rw-accent);
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
    color: var(--rw-accent);
  }
</style>
