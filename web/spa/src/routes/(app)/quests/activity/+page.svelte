<script lang="ts">
  import {
    listLocationActivity,
    getLocation,
    ApiClientError,
    type ActivityEntryShape,
    type LocationShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';
  import { goto } from '$app/navigation';

  // Climber-facing activity feed for the selected location. Mirrors the
  // HTMX /quests/activity page (climber/quest-activity.html). Climbers
  // bounce when progressions are off; staff stay so they can preview
  // before the feature flag flips.

  let entries = $state<ActivityEntryShape[]>([]);
  let location = $state<LocationShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());
  const enabled = $derived(location ? !!location.progressions_enabled : null);
  const isStaff = $derived(roleRankAt(locId) >= 2);

  $effect(() => {
    if (enabled === false && !isStaff) {
      goto('/');
    }
  });

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getLocation(locId),
      listLocationActivity(locId, { limit: 50 }).catch(() => [] as ActivityEntryShape[]),
    ])
      .then(([loc, list]) => {
        if (cancelled) return;
        location = loc;
        entries = list;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load activity.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function fmtAge(iso: string): string {
    const ms = Date.now() - new Date(iso).getTime();
    const sec = Math.floor(ms / 1000);
    if (sec < 60) return `${sec}s ago`;
    const min = Math.floor(sec / 60);
    if (min < 60) return `${min}m ago`;
    const hr = Math.floor(min / 60);
    if (hr < 24) return `${hr}h ago`;
    return new Date(iso).toLocaleDateString();
  }

  // Cheap, readable summary. The metadata blob is denormalized at write
  // time so we don't have to look anything up; just pull whatever fields
  // the activity_type usually carries. Falls back to "did X" when an
  // unknown type lands.
  function summarize(e: ActivityEntryShape): string {
    const md = e.metadata ?? {};
    const get = (k: string) => (typeof md[k] === 'string' ? (md[k] as string) : '');
    switch (e.activity_type) {
      case 'quest_started':
        return `started ${get('quest_name') || 'a quest'}`;
      case 'quest_completed':
        return `completed ${get('quest_name') || 'a quest'}`;
      case 'badge_earned':
        return `earned ${get('badge_name') || 'a badge'}`;
      case 'route_set':
      case 'route_published':
        return `set ${get('route_grade') || 'a route'}${get('route_name') ? ' — ' + get('route_name') : ''}`;
      case 'ascent_logged': {
        const t = get('ascent_type') || 'logged';
        return `${t} ${get('route_grade') || 'a route'}${get('route_name') ? ' — ' + get('route_name') : ''}`;
      }
      default:
        return `did ${e.activity_type.replace(/_/g, ' ')}`;
    }
  }
</script>

<svelte:head>
  <title>Activity — Routewerk</title>
</svelte:head>

<div class="page">
  <p><a class="back" href="/quests">← Quests</a></p>
  <header class="page-header">
    <h1>Activity</h1>
    <p class="lede">Most recent climbs, sets, quest progress, and badges at this gym.</p>
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar to see its activity.</p>
  {:else if loading}
    <p class="muted">Loading activity…</p>
  {:else if enabled === false}
    <div class="empty-card">
      <h3>Progressions aren't enabled here yet</h3>
      <p>
        The climber-facing feed lights up once a gym manager turns
        progressions on under
        <a class="link" href="/settings/gym">gym settings</a>.
      </p>
    </div>
  {:else if error}
    <p class="error">{error}</p>
  {:else if entries.length === 0}
    <div class="empty-card">
      <h3>No activity yet</h3>
      <p>Send something or set a route to seed the feed.</p>
    </div>
  {:else}
    <ul class="feed">
      {#each entries as e (e.id)}
        <li>
          <span class="avatar-fallback">{(e.user_display_name?.[0] ?? '?').toUpperCase()}</span>
          <div class="entry-body">
            <span class="user">{e.user_display_name ?? 'Someone'}</span>
            <span class="action">{summarize(e)}</span>
          </div>
          <span class="age muted">{fmtAge(e.created_at)}</span>
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
  .feed {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .feed li {
    display: grid;
    grid-template-columns: 32px 1fr auto;
    gap: 12px;
    align-items: center;
    padding: 0.55rem 0.85rem;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 8px;
  }
  .avatar-fallback {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.78rem;
  }
  .entry-body {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    flex-wrap: wrap;
    font-size: 0.92rem;
  }
  .user {
    font-weight: 600;
  }
  .action {
    color: var(--rw-text-muted);
  }
  .age {
    font-size: 0.78rem;
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
  .empty-card .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
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
