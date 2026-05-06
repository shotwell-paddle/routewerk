<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import {
    getSession,
    listSessionRoutes,
    uploadRoutePhoto,
    ApiClientError,
    type SessionShape,
    type SessionRouteDetailShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { authState, roleRankAt } from '$lib/stores/auth.svelte';

  // Per-session photo dashboard. Mirrors the HTMX setter/session-photos.html
  // page: shows every route in the session as a card, lets the setter
  // upload directly from here. Each upload reuses the existing route
  // photo endpoint (#81) so the row shows up in /app/routes/[id] too.

  const sessionId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());
  const canManage = $derived(roleRankAt(locId) >= 2);

  let session = $state<SessionShape | null>(null);
  let routes = $state<SessionRouteDetailShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Per-route upload tracking. Two routes can upload in parallel
  // without stomping each other since the busy flag is keyed by id.
  let uploadingId = $state<string | null>(null);
  let uploadError = $state<string | null>(null);

  // Page-level role gate — climbers + signed-out users bounce.
  $effect(() => {
    if (!authState().loaded || !authState().me) return;
    if (!canManage) {
      goto('/app');
    }
  });

  async function refresh() {
    if (!locId || !sessionId) return;
    const [s, rs] = await Promise.all([
      getSession(locId, sessionId),
      listSessionRoutes(locId, sessionId),
    ]);
    session = s;
    routes = rs;
  }

  $effect(() => {
    if (!locId || !sessionId) return;
    let cancelled = false;
    loading = true;
    error = null;
    refresh()
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load session.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function onPhotoFile(routeId: string, e: Event) {
    if (!locId) return;
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    uploadingId = routeId;
    uploadError = null;
    try {
      await uploadRoutePhoto(locId, routeId, file);
      // Re-fetch routes so the photo_url updates inline.
      routes = await listSessionRoutes(locId, sessionId);
      input.value = '';
    } catch (err) {
      uploadError = err instanceof ApiClientError ? err.message : 'Upload failed.';
    } finally {
      uploadingId = null;
    }
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
      year: 'numeric',
    });
  }

  // Aggregate stats so the setter can see "12 / 18 routes have photos"
  // without counting cards manually.
  const photoCount = $derived(routes.filter((r) => r.photo_url).length);
  const photoPercent = $derived(
    routes.length === 0 ? 0 : Math.round((photoCount * 100) / routes.length),
  );
</script>

<svelte:head>
  <title>Session photos — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/sessions/{sessionId}">← Session</a>
  <header class="page-header">
    <div>
      <h1>Photos {session ? `— ${fmtDate(session.scheduled_date)}` : ''}</h1>
      <p class="lede">
        Upload a card photo for every route in this session — climbers see
        them on the route detail page.
      </p>
    </div>
    {#if routes.length > 0}
      <div class="progress">
        <div class="progress-bar">
          <span class="fill" style="width:{photoPercent}%"></span>
        </div>
        <span class="muted small">{photoCount} / {routes.length} ({photoPercent}%)</span>
      </div>
    {/if}
  </header>

  {#if !locId || !sessionId}
    <p class="muted">Pick a session.</p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if routes.length === 0}
    <div class="empty-card">
      <h3>No routes set yet</h3>
      <p>
        Once routes are linked to this session (during the setting day),
        they show up here for photo upload.
      </p>
    </div>
  {:else}
    {#if uploadError}<p class="error">{uploadError}</p>{/if}
    <ul class="route-grid">
      {#each routes as r (r.id)}
        <li class="route-card" class:has-photo={!!r.photo_url}>
          <div class="thumb">
            {#if r.photo_url}
              <img src={r.photo_url} alt="" loading="lazy" />
            {:else}
              <span class="placeholder muted small">No photo</span>
            {/if}
          </div>
          <div class="meta">
            <div class="head">
              <span class="color-chip" style="background:{r.color}"></span>
              <span class="grade">{r.grade}</span>
              {#if r.name}<span class="rname">— {r.name}</span>{/if}
            </div>
            <p class="muted small">
              {r.wall_name}
              {#if r.setter_name}· set by {r.setter_name}{/if}
            </p>
            <label class="upload-trigger">
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp"
                onchange={(e) => onPhotoFile(r.id, e)}
                disabled={uploadingId === r.id} />
              <span>
                {uploadingId === r.id
                  ? 'Uploading…'
                  : r.photo_url
                  ? 'Replace photo'
                  : '+ Add photo'}
              </span>
            </label>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .page {
    max-width: 64rem;
  }
  .back {
    display: inline-block;
    color: var(--rw-text-muted);
    text-decoration: none;
    font-size: 0.9rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }
  .back:hover {
    color: var(--rw-accent);
  }
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-end;
    gap: 1rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
    max-width: 36rem;
  }
  .progress {
    display: flex;
    flex-direction: column;
    gap: 4px;
    align-items: flex-end;
  }
  .progress-bar {
    width: 12rem;
    height: 8px;
    border-radius: 999px;
    background: var(--rw-surface-alt);
    overflow: hidden;
  }
  .progress-bar .fill {
    display: block;
    height: 100%;
    background: var(--rw-accent);
  }
  .small {
    font-size: 0.78rem;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .route-grid {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr));
    gap: 12px;
  }
  .route-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  .route-card.has-photo {
    border-color: var(--rw-accent);
  }
  .thumb {
    aspect-ratio: 4 / 3;
    background: var(--rw-surface-alt);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .placeholder {
    font-style: italic;
  }
  .meta {
    padding: 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .head {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .color-chip {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .grade {
    font-weight: 700;
  }
  .rname {
    color: var(--rw-text);
  }
  .upload-trigger {
    align-self: flex-start;
    cursor: pointer;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-radius: 6px;
    padding: 0.4rem 0.85rem;
    font-size: 0.85rem;
    font-weight: 600;
    margin-top: 4px;
  }
  .upload-trigger:hover {
    background: var(--rw-accent-hover);
  }
  .upload-trigger input[type='file'] {
    display: none;
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
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
    margin: 0 0 1rem;
  }
</style>
