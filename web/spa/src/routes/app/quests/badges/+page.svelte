<script lang="ts">
  import {
    getBadgeShowcase,
    getLocation,
    ApiClientError,
    type BadgeShowcaseEntry,
    type LocationShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  // Climber-facing badge collection — earned cards lit up, unearned
  // cards greyed. Mirrors the HTMX /quests/badges page (template at
  // web/templates/climber/badge-showcase.html). Like the rest of the
  // progressions surface, gated to locations with progressions
  // enabled — staff can still preview when off so they can populate
  // the catalog before flipping the flag.

  let badges = $state<BadgeShowcaseEntry[]>([]);
  let location = $state<LocationShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());
  const enabled = $derived(location ? !!location.progressions_enabled : null);

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getLocation(locId),
      getBadgeShowcase(locId).catch(() => [] as BadgeShowcaseEntry[]),
    ])
      .then(([loc, b]) => {
        if (cancelled) return;
        location = loc;
        badges = b;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load badges.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  // Earned first, then alphabetical so the climber's wins land at the top.
  const ordered = $derived([...badges].sort((a, b) => {
    if (!!a.earned_at !== !!b.earned_at) return a.earned_at ? -1 : 1;
    return a.name.localeCompare(b.name);
  }));

  const earnedCount = $derived(badges.filter((b) => !!b.earned_at).length);

  function fmtDate(iso: string | undefined): string {
    if (!iso) return '';
    return new Date(iso).toLocaleDateString();
  }
</script>

<svelte:head>
  <title>Badges — Routewerk</title>
</svelte:head>

<div class="page">
  <p><a class="back" href="/app/quests">← Quests</a></p>
  <header class="page-header">
    <h1>Badges</h1>
    <p class="lede">
      {#if badges.length > 0}
        {earnedCount} of {badges.length} earned at this gym.
      {:else}
        Your collection of badges from this gym.
      {/if}
    </p>
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar to see its badges.</p>
  {:else if loading}
    <p class="muted">Loading badges…</p>
  {:else if enabled === false}
    <div class="empty-card">
      <h3>Badges aren't enabled here yet</h3>
      <p>
        This location hasn't turned on the progressions feature. A gym manager
        can flip it on in gym settings.
      </p>
    </div>
  {:else if error}
    <p class="error">{error}</p>
  {:else if badges.length === 0}
    <div class="empty-card">
      <h3>No badges yet</h3>
      <p>This gym hasn't published any badges. Check back later.</p>
    </div>
  {:else}
    <ul class="badge-grid">
      {#each ordered as b (b.id)}
        <li class="badge-card" class:earned={!!b.earned_at}>
          <div class="badge-icon" style="background:{b.color}">{b.icon}</div>
          <div class="badge-meta">
            <h3>{b.name}</h3>
            {#if b.description}<p class="desc muted">{b.description}</p>{/if}
            {#if b.earned_at}
              <p class="earned-line">Earned {fmtDate(b.earned_at)}</p>
            {:else}
              <p class="locked-line muted">Not yet earned</p>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
  }
  .back {
    color: var(--rw-text-muted);
    text-decoration: none;
    font-size: 0.9rem;
    font-weight: 600;
  }
  .back:hover {
    color: var(--rw-accent);
  }
  .page-header {
    margin: 0.25rem 0 1.5rem;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
  }
  .badge-grid {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: 12px;
  }
  .badge-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1rem;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 12px;
    align-items: start;
    opacity: 0.55;
    transition: opacity 120ms;
  }
  .badge-card.earned {
    opacity: 1;
    border-color: var(--rw-accent);
  }
  .badge-icon {
    width: 56px;
    height: 56px;
    border-radius: 50%;
    color: #fff;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 1.6rem;
    flex-shrink: 0;
    font-weight: 700;
  }
  .badge-card:not(.earned) .badge-icon {
    filter: grayscale(0.7);
  }
  .badge-meta h3 {
    margin: 0 0 4px;
    font-size: 1rem;
    font-weight: 700;
  }
  .desc {
    margin: 0 0 6px;
    font-size: 0.85rem;
  }
  .earned-line {
    color: var(--rw-accent);
    margin: 0;
    font-size: 0.8rem;
    font-weight: 600;
  }
  .locked-line {
    margin: 0;
    font-size: 0.8rem;
    font-style: italic;
  }
  .empty-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 2.5rem 1.5rem;
    text-align: center;
  }
  .empty-card h3 {
    margin: 0 0 0.4rem;
    font-size: 1.15rem;
  }
  .empty-card p {
    color: var(--rw-text-muted);
    margin: 0;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
</style>
