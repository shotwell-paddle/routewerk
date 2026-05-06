<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import {
    getRoute,
    listWalls,
    listRouteAscents,
    listRouteRatings,
    logAscent,
    rateRoute,
    updateRoute,
    updateRouteStatus,
    deleteRoute,
    getLocationSettings,
    listRoutePhotos,
    uploadRoutePhoto,
    deleteRoutePhoto,
    listCommunityTags,
    addCommunityTag,
    removeCommunityTag,
    moderateCommunityTag,
    getRouteDifficulty,
    voteRouteDifficulty,
    ApiClientError,
    type RouteShape,
    type RouteStatus,
    type RouteWriteShape,
    type WallShape,
    type AscentShape,
    type RouteRatingShape,
    type AscentType,
    type LocationSettingsShape,
    type RoutePhotoShape,
    type CommunityTagShape,
    type DifficultyConsensusShape,
    type DifficultyVote,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';
  import { authState } from '$lib/stores/auth.svelte';
  import RouteForm from '$lib/components/RouteForm.svelte';

  let route = $state<RouteShape | null>(null);
  let walls = $state<WallShape[]>([]);
  let settings = $state<LocationSettingsShape | null>(null);
  let ascents = $state<AscentShape[]>([]);
  let ratings = $state<RouteRatingShape[]>([]);
  let photos = $state<RoutePhotoShape[]>([]);
  let communityTags = $state<CommunityTagShape[]>([]);
  let consensus = $state<DifficultyConsensusShape | null>(null);

  // New-tag input + per-row mutating state. Two requests can fly in
  // parallel without stomping each other because we key the loading
  // flag by tag name.
  let tagInput = $state('');
  let tagSubmitting = $state(false);
  let tagError = $state<string | null>(null);
  let tagBusy = $state<string | null>(null);

  let voteSubmitting = $state(false);
  let voteError = $state<string | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Photo upload state — file picker + lightbox + per-photo delete.
  let photoUploading = $state(false);
  let photoError = $state<string | null>(null);
  let lightboxIdx = $state<number | null>(null);

  let editing = $state(false);
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let statusUpdating = $state(false);
  let deleting = $state(false);

  // Climber-side action state — both posts back to the same endpoints
  // the HTMX climber detail uses, so behaviour is identical with the
  // existing app.
  let logForm = $state({
    ascent_type: 'send' as AscentType,
    attempts: 1,
    notes: '',
  });
  let logSaving = $state(false);
  let logError = $state<string | null>(null);
  let logOk = $state<string | null>(null);

  let rateForm = $state({ rating: 5, comment: '' });
  let rateSaving = $state(false);
  let rateError = $state<string | null>(null);
  let rateOk = $state<string | null>(null);

  const routeId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());
  // Setter+ may edit + flip status; head_setter+ may delete. Server enforces
  // both — UI hides the affordances so climbers don't see actionable cards
  // they can't actually use.
  const canEdit = $derived(roleRankAt(locId) >= 2);
  const canDelete = $derived(roleRankAt(locId) >= 3);

  const wallName = $derived.by(() => {
    if (!route) return '—';
    return walls.find((w) => w.id === route!.wall_id)?.name ?? '—';
  });

  $effect(() => {
    if (!locId || !routeId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getRoute(locId, routeId),
      listWalls(locId),
      // Ascents + ratings are best-effort — failures shouldn't block the
      // detail render, just leave the panels empty.
      listRouteAscents(locId, routeId, 20).catch(() => [] as AscentShape[]),
      listRouteRatings(locId, routeId).catch(() => [] as RouteRatingShape[]),
      // Settings drive the edit form's pickers — best-effort; permission
      // failure leaves the form on default lists.
      getLocationSettings(locId).catch(() => null),
      listRoutePhotos(locId, routeId).catch(() => [] as RoutePhotoShape[]),
      listCommunityTags(locId, routeId).catch(() => [] as CommunityTagShape[]),
      getRouteDifficulty(locId, routeId).catch(() => null),
    ])
      .then(([r, wls, asc, rt, st, ph, tg, cons]) => {
        if (cancelled) return;
        route = r;
        walls = wls;
        ascents = asc;
        ratings = rt;
        settings = st;
        photos = ph;
        communityTags = tg;
        consensus = cons;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load route.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function submitEdit(body: RouteWriteShape) {
    if (!locId || !routeId) return;
    saving = true;
    saveError = null;
    try {
      route = await updateRoute(locId, routeId, body);
      editing = false;
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      saving = false;
    }
  }

  async function setStatus(s: RouteStatus) {
    if (!locId || !routeId || !route || route.status === s) return;
    statusUpdating = true;
    saveError = null;
    try {
      route = await updateRouteStatus(locId, routeId, s);
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Status change failed.';
    } finally {
      statusUpdating = false;
    }
  }

  async function handleDelete() {
    if (!locId || !routeId || !route) return;
    if (!confirm(`Permanently delete this route? This cannot be undone.`)) return;
    deleting = true;
    try {
      await deleteRoute(locId, routeId);
      goto('/routes');
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Could not delete route.';
      deleting = false;
    }
  }

  // Climber tick — POST to the same endpoint the HTMX page uses. After a
  // successful log, refresh the route + ascents so counts and feed update.
  async function submitLog(e: Event) {
    e.preventDefault();
    if (!locId || !routeId) return;
    logSaving = true;
    logError = null;
    logOk = null;
    try {
      await logAscent(locId, routeId, {
        ascent_type: logForm.ascent_type,
        attempts: Math.max(1, Math.floor(logForm.attempts)),
        notes: logForm.notes.trim() || null,
      });
      // Refresh fresh route + ascents in parallel.
      const [r, asc] = await Promise.all([
        getRoute(locId, routeId),
        listRouteAscents(locId, routeId, 20).catch(() => ascents),
      ]);
      route = r;
      ascents = asc;
      logOk = 'Tick logged.';
      logForm = { ascent_type: 'send', attempts: 1, notes: '' };
    } catch (err) {
      logError = err instanceof ApiClientError ? err.message : 'Could not log tick.';
    } finally {
      logSaving = false;
    }
  }

  // Climber rate — server upserts on (user, route), so resubmitting just
  // overwrites the previous rating.
  async function submitRate(e: Event) {
    e.preventDefault();
    if (!locId || !routeId) return;
    if (rateForm.rating < 1 || rateForm.rating > 5) {
      rateError = 'Pick 1–5 stars.';
      return;
    }
    rateSaving = true;
    rateError = null;
    rateOk = null;
    try {
      await rateRoute(locId, routeId, {
        rating: rateForm.rating,
        comment: rateForm.comment.trim() || null,
      });
      const [r, rt] = await Promise.all([
        getRoute(locId, routeId),
        listRouteRatings(locId, routeId).catch(() => ratings),
      ]);
      route = r;
      ratings = rt;
      rateOk = 'Rating saved.';
    } catch (err) {
      rateError = err instanceof ApiClientError ? err.message : 'Could not save rating.';
    } finally {
      rateSaving = false;
    }
  }

  // Pre-fill the rating form with the user's prior rating if one exists.
  // The list returns ratings descending; pick the first whose user_id
  // matches.
  $effect(() => {
    const meId = authState().me?.user.id;
    if (!meId) return;
    const mine = ratings.find((r) => r.user_id === meId);
    if (mine) {
      rateForm = { rating: mine.rating, comment: mine.comment ?? '' };
    }
  });

  // Did the user already log an ascent on this route? Drives the "logged"
  // hint above the log form so they don't re-tick by accident.
  const myAscent = $derived.by(() => {
    const meId = authState().me?.user.id;
    if (!meId) return null;
    return ascents.find((a) => a.user_id === meId) ?? null;
  });

  // Photo upload — multipart POST. After success, re-list photos so the
  // new one shows up; the server may have set route.photo_url too if this
  // was the first photo, so refetch the route in the same pass.
  async function onPhotoFile(e: Event) {
    if (!locId || !routeId) return;
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    photoUploading = true;
    photoError = null;
    try {
      await uploadRoutePhoto(locId, routeId, file);
      const [r, ph] = await Promise.all([
        getRoute(locId, routeId).catch(() => route),
        listRoutePhotos(locId, routeId).catch(() => photos),
      ]);
      route = r;
      photos = ph;
      input.value = '';
    } catch (err) {
      photoError = err instanceof ApiClientError ? err.message : 'Upload failed.';
    } finally {
      photoUploading = false;
    }
  }

  async function removePhoto(p: RoutePhotoShape) {
    if (!locId || !routeId) return;
    if (!confirm('Delete this photo?')) return;
    photoError = null;
    try {
      await deleteRoutePhoto(locId, routeId, p.id);
      const [r, ph] = await Promise.all([
        getRoute(locId, routeId).catch(() => route),
        listRoutePhotos(locId, routeId).catch(() => photos),
      ]);
      route = r;
      photos = ph;
    } catch (err) {
      photoError = err instanceof ApiClientError ? err.message : 'Delete failed.';
    }
  }

  // The current user can delete a photo if they uploaded it OR if they're
  // setter+ at the location. Mirrors the server's authz at
  // internal/handler/routephoto.go::Delete.
  function canDeletePhoto(p: RoutePhotoShape): boolean {
    const meId = authState().me?.user.id;
    if (!meId) return false;
    if (p.uploaded_by === meId) return true;
    return roleRankAt(locId) >= 2;
  }

  // Community tag handlers. The server returns the updated aggregated
  // list on add/remove so we don't need a separate refetch.
  async function submitTag(e: Event) {
    e.preventDefault();
    if (!locId || !routeId) return;
    const v = tagInput.trim();
    if (!v) return;
    tagSubmitting = true;
    tagError = null;
    try {
      communityTags = await addCommunityTag(locId, routeId, v);
      tagInput = '';
    } catch (err) {
      tagError = err instanceof ApiClientError ? err.message : 'Could not add tag.';
    } finally {
      tagSubmitting = false;
    }
  }

  async function toggleTag(tag: CommunityTagShape) {
    if (!locId || !routeId || tagBusy) return;
    tagBusy = tag.tag_name;
    tagError = null;
    try {
      communityTags = tag.user_added
        ? await removeCommunityTag(locId, routeId, tag.tag_name)
        : await addCommunityTag(locId, routeId, tag.tag_name);
    } catch (err) {
      tagError = err instanceof ApiClientError ? err.message : 'Could not update tag.';
    } finally {
      tagBusy = null;
    }
  }

  async function moderateTag(tag: CommunityTagShape) {
    if (!locId || !routeId || tagBusy) return;
    if (!confirm(`Remove every "${tag.tag_name}" vote from this route? Climbers won't be able to see it anymore.`)) return;
    tagBusy = tag.tag_name;
    tagError = null;
    try {
      await moderateCommunityTag(locId, routeId, tag.tag_name);
      communityTags = await listCommunityTags(locId, routeId);
    } catch (err) {
      tagError = err instanceof ApiClientError ? err.message : 'Could not delete tag.';
    } finally {
      tagBusy = null;
    }
  }

  // head_setter+ may scrub a tag from the route entirely (matches the
  // HTMX moderation endpoint). The server enforces the same gate.
  const canModerateTags = $derived(roleRankAt(locId) >= 3);

  async function submitVote(v: DifficultyVote) {
    if (!locId || !routeId || voteSubmitting) return;
    voteSubmitting = true;
    voteError = null;
    try {
      consensus = await voteRouteDifficulty(locId, routeId, v);
    } catch (err) {
      voteError = err instanceof ApiClientError ? err.message : 'Could not save vote.';
    } finally {
      voteSubmitting = false;
    }
  }

  function fmtDate(iso: string | null | undefined): string {
    if (!iso) return '—';
    return new Date(iso).toLocaleDateString();
  }

  function fmtDateTime(iso: string): string {
    return new Date(iso).toLocaleString();
  }

  // Ratings histogram counts for 1–5.
  const histogram = $derived.by(() => {
    const counts = [0, 0, 0, 0, 0];
    for (const r of ratings) {
      if (r.rating >= 1 && r.rating <= 5) counts[r.rating - 1]++;
    }
    return counts;
  });
  const histogramMax = $derived(Math.max(1, ...histogram));
</script>

<svelte:head>
  <title>{route ? `${route.grade} ${route.name ?? ''}` : 'Route'} — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/routes">← Routes</a>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if route}
    <header class="page-header">
      <div class="title-block">
        <span class="color-chip" style="background:{route.color}" title={route.color}></span>
        <div>
          <div class="title-meta">
            <span class="grade">{route.grade}</span>
            {#if route.name}<span class="name">{route.name}</span>{/if}
          </div>
          <p class="meta-line muted">
            {wallName} · {route.route_type} · set {fmtDate(route.date_set)}
            {#if route.projected_strip_date}· strip target {fmtDate(route.projected_strip_date)}{/if}
          </p>
        </div>
      </div>
      {#if !editing && canEdit}
        <div class="header-actions">
          <button onclick={() => (editing = true)}>Edit</button>
        </div>
      {/if}
    </header>

    {#if editing}
      <RouteForm
        initial={route}
        {walls}
        {settings}
        submitLabel="Save changes"
        onSubmit={submitEdit}
        onCancel={() => {
          editing = false;
          saveError = null;
        }}
        {saving}
        error={saveError} />
    {:else}
      <div class="grid">
        <section class="card status-card">
          <h2>Status</h2>
          <p class="status-current">
            Currently <span class="status-pill status-{route.status}">{route.status}</span>
          </p>
          {#if canEdit}
            <div class="status-actions">
              {#each ['active', 'flagged', 'archived'] as RouteStatus[] as s}
                <button
                  disabled={statusUpdating || route.status === s}
                  class:active={route.status === s}
                  onclick={() => setStatus(s)}>
                  {s}
                </button>
              {/each}
            </div>
            {#if saveError}<p class="error">{saveError}</p>{/if}
          {/if}
        </section>

        <section class="card stats-card">
          <h2>Stats</h2>
          <dl>
            <div>
              <dt>Avg rating</dt>
              <dd>{route.avg_rating > 0 ? route.avg_rating.toFixed(1) : '—'}</dd>
            </div>
            <div><dt>Ratings</dt><dd>{route.rating_count}</dd></div>
            <div><dt>Sends</dt><dd>{route.ascent_count}</dd></div>
            <div><dt>Attempts</dt><dd>{route.attempt_count}</dd></div>
          </dl>
        </section>
      </div>

      {#if route.description}
        <section class="card">
          <h2>Description</h2>
          <p class="prose">{route.description}</p>
        </section>
      {/if}

      <section class="card">
        <div class="card-head">
          <h2>Photos {photos.length > 0 ? `(${photos.length})` : ''}</h2>
          {#if route.status !== 'archived' && photos.length < 20}
            <label class="upload-trigger">
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp"
                onchange={onPhotoFile}
                disabled={photoUploading} />
              <span>{photoUploading ? 'Uploading…' : '+ Add photo'}</span>
            </label>
          {/if}
        </div>
        {#if photoError}<p class="error">{photoError}</p>{/if}
        {#if photos.length === 0}
          {#if route.photo_url}
            <!-- Legacy: route has a primary URL but no row in route_photos yet
                 (e.g. set via the HTMX form pre-photo-row migration). Show it
                 so the page isn't visually empty. -->
            <img src={route.photo_url} alt="" class="photo" />
          {:else}
            <p class="muted">No photos yet — add one to help climbers spot the route on the wall.</p>
          {/if}
        {:else}
          <ul class="photo-grid">
            {#each photos as p, i (p.id)}
              <li>
                <button
                  type="button"
                  class="photo-thumb"
                  onclick={() => (lightboxIdx = i)}
                  aria-label="View photo">
                  <img src={p.photo_url} alt="" loading="lazy" />
                </button>
                {#if p.uploader_name}
                  <span class="photo-meta muted">by {p.uploader_name}</span>
                {/if}
                {#if canDeletePhoto(p)}
                  <button
                    type="button"
                    class="photo-delete"
                    onclick={() => removePhoto(p)}
                    aria-label="Delete photo">
                    ×
                  </button>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      {#if lightboxIdx !== null && photos[lightboxIdx]}
        <button
          type="button"
          class="lightbox"
          onclick={() => (lightboxIdx = null)}
          aria-label="Close photo">
          <img src={photos[lightboxIdx].photo_url} alt="" />
        </button>
      {/if}

      <section class="card">
        <h2>Community tags</h2>
        <p class="muted small">
          What climbers say this route feels like. Click a tag to vote it up
          or back out; add your own below.
        </p>
        {#if communityTags.length === 0}
          <p class="muted">No tags yet — be the first.</p>
        {:else}
          <ul class="tag-list">
            {#each communityTags as tag (tag.tag_name)}
              <li>
                <button
                  type="button"
                  class="tag-chip"
                  class:voted={tag.user_added}
                  disabled={tagBusy === tag.tag_name}
                  onclick={() => toggleTag(tag)}
                  title={tag.user_added ? 'You voted for this — click to remove your vote' : 'Vote for this tag'}>
                  {tag.tag_name}
                  <span class="tag-count">{tag.count}</span>
                </button>
                {#if canModerateTags}
                  <button
                    type="button"
                    class="tag-mod"
                    disabled={tagBusy === tag.tag_name}
                    onclick={() => moderateTag(tag)}
                    title="Remove this tag from the route entirely (moderator)">
                    ×
                  </button>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}

        {#if route.status !== 'archived'}
          <form class="tag-form" onsubmit={submitTag}>
            <input
              type="text"
              bind:value={tagInput}
              maxlength="30"
              placeholder="add a tag (e.g. crimpy, slab, pumpy)…"
              disabled={tagSubmitting} />
            <button class="primary" type="submit" disabled={tagSubmitting || !tagInput.trim()}>
              {tagSubmitting ? '…' : 'Add'}
            </button>
          </form>
        {/if}
        {#if tagError}<p class="error tag-err">{tagError}</p>{/if}
      </section>

      {#if consensus}
        <section class="card">
          <h2>Difficulty consensus</h2>
          {#if consensus.total_votes === 0}
            <p class="muted">
              No one has rated the difficulty yet. Vote so the next climber knows
              what to expect for {route.grade}.
            </p>
          {:else}
            <div class="consensus-bar" aria-label="Vote breakdown">
              {#if consensus.easy_pct > 0}
                <span class="seg easy" style="width: {consensus.easy_pct}%" title="{consensus.easy_count} climbers said easier"></span>
              {/if}
              {#if consensus.right_pct > 0}
                <span class="seg right" style="width: {consensus.right_pct}%" title="{consensus.right_count} climbers said right on"></span>
              {/if}
              {#if consensus.hard_pct > 0}
                <span class="seg hard" style="width: {consensus.hard_pct}%" title="{consensus.hard_count} climbers said harder"></span>
              {/if}
            </div>
            <p class="muted small consensus-counts">
              <span class="dot easy" aria-hidden="true"></span>{consensus.easy_count} easier ·
              <span class="dot right" aria-hidden="true"></span>{consensus.right_count} right on ·
              <span class="dot hard" aria-hidden="true"></span>{consensus.hard_count} harder
              ({consensus.total_votes} {consensus.total_votes === 1 ? 'vote' : 'votes'})
            </p>
          {/if}
          {#if route.status !== 'archived'}
            <div class="vote-row">
              <span class="muted small">Your vote:</span>
              {#each [['easy', 'Easier'], ['right', 'Right on'], ['hard', 'Harder']] as [v, label]}
                <button
                  type="button"
                  class="vote-btn vote-{v}"
                  class:active={consensus.my_vote === v}
                  disabled={voteSubmitting}
                  onclick={() => submitVote(v as DifficultyVote)}>
                  {label}
                </button>
              {/each}
            </div>
            {#if voteError}<p class="error tag-err">{voteError}</p>{/if}
          {/if}
        </section>
      {/if}

      {#if route.status !== 'archived'}
        <section class="card climber-actions">
          <h2>Log an ascent</h2>
          {#if myAscent}
            <p class="muted prior-tick">
              You already logged a <strong>{myAscent.ascent_type}</strong>
              on {fmtDate(myAscent.climbed_at)}. Logging again adds a
              second entry.
            </p>
          {/if}
          <form onsubmit={submitLog}>
            <div class="action-row">
              <fieldset class="ascent-type-group">
                <legend class="sr-only">Ascent type</legend>
                {#each ['send', 'flash', 'attempt'] as AscentType[] as t}
                  <label class="seg" class:active={logForm.ascent_type === t}>
                    <input
                      type="radio"
                      name="ascent_type"
                      value={t}
                      bind:group={logForm.ascent_type} />
                    {t}
                  </label>
                {/each}
              </fieldset>
              <label class="attempts-field">
                <span>Attempts</span>
                <input
                  type="number"
                  min="1"
                  step="1"
                  bind:value={logForm.attempts} />
              </label>
            </div>
            <label class="notes-field">
              <span>Notes (optional)</span>
              <textarea
                bind:value={logForm.notes}
                rows="2"
                placeholder="Beta, conditions, anything memorable…"></textarea>
            </label>
            {#if logError}<p class="error">{logError}</p>{/if}
            {#if logOk}<p class="ok">{logOk}</p>{/if}
            <button class="primary" type="submit" disabled={logSaving}>
              {logSaving ? 'Logging…' : 'Log it'}
            </button>
          </form>
        </section>

        <section class="card climber-actions">
          <h2>Rate this route</h2>
          <form onsubmit={submitRate}>
            <fieldset class="star-group">
              <legend class="sr-only">Stars</legend>
              {#each [1, 2, 3, 4, 5] as n}
                <label class="star" class:filled={rateForm.rating >= n}>
                  <input
                    type="radio"
                    name="rating"
                    value={n}
                    bind:group={rateForm.rating} />
                  ★
                </label>
              {/each}
            </fieldset>
            <label class="notes-field">
              <span>Comment (optional)</span>
              <textarea
                bind:value={rateForm.comment}
                rows="2"
                placeholder="What worked, what didn't…"></textarea>
            </label>
            {#if rateError}<p class="error">{rateError}</p>{/if}
            {#if rateOk}<p class="ok">{rateOk}</p>{/if}
            <button class="primary" type="submit" disabled={rateSaving}>
              {rateSaving ? 'Saving…' : 'Save rating'}
            </button>
          </form>
        </section>
      {/if}

      {#if ratings.length > 0}
        <section class="card">
          <h2>Ratings ({ratings.length})</h2>
          <div class="histogram">
            {#each [5, 4, 3, 2, 1] as star}
              {@const count = histogram[star - 1]}
              <div class="hist-row">
                <span class="hist-label">{star}★</span>
                <div class="hist-bar">
                  <span class="hist-fill" style="width: {(count / histogramMax) * 100}%"></span>
                </div>
                <span class="hist-count">{count}</span>
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <section class="card">
        <h2>Recent ascents</h2>
        {#if ascents.length === 0}
          <p class="muted">No ascents logged yet.</p>
        {:else}
          <ul class="ascent-list">
            {#each ascents as a (a.id)}
              <li>
                <span class="ascent-type ascent-{a.ascent_type}">{a.ascent_type}</span>
                <span class="ascent-meta muted">
                  {fmtDateTime(a.climbed_at)} · {a.attempts} attempt{a.attempts === 1 ? '' : 's'}
                </span>
                {#if a.notes}<span class="ascent-notes">{a.notes}</span>{/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      {#if canDelete}
      <section class="card danger-zone">
        <h2>Danger zone</h2>
        <p class="muted">
          Permanent delete (head_setter+). Prefer "archived" status if you
          want to hide the route without losing its history.
        </p>
        <button class="danger" disabled={deleting} onclick={handleDelete}>
          {deleting ? 'Deleting…' : 'Delete route'}
        </button>
      </section>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
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
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }
  .title-block {
    display: flex;
    align-items: center;
    gap: 0.85rem;
  }
  .color-chip {
    width: 36px;
    height: 36px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .title-meta {
    display: inline-flex;
    align-items: baseline;
    gap: 8px;
  }
  .grade {
    font-size: 1.4rem;
    font-weight: 800;
    letter-spacing: -0.01em;
  }
  .name {
    font-size: 1.05rem;
    color: var(--rw-text);
  }
  .meta-line {
    margin: 4px 0 0;
    font-size: 0.85rem;
  }
  .header-actions {
    display: flex;
    gap: 8px;
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(16rem, 1fr));
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .card h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.75rem;
  }
  .status-current {
    margin: 0 0 0.75rem;
    color: var(--rw-text-muted);
    font-size: 0.9rem;
  }
  .status-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 1px 8px;
    border-radius: 4px;
    font-weight: 700;
    margin-left: 4px;
  }
  .status-active {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-flagged {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .status-archived {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .status-actions {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .status-actions button.active {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
  }
  .stats-card dl {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 0.75rem 1.5rem;
  }
  .stats-card dt {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--rw-text-faint);
    margin: 0 0 2px;
  }
  .stats-card dd {
    margin: 0;
    color: var(--rw-text);
    font-weight: 600;
    font-size: 1.1rem;
  }
  .prose {
    margin: 0;
    color: var(--rw-text);
    line-height: 1.5;
    white-space: pre-wrap;
  }
  .photo {
    max-width: 100%;
    border-radius: 8px;
    border: 1px solid var(--rw-border);
  }
  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 8px;
  }
  .card-head h2 {
    margin: 0;
  }
  .upload-trigger {
    cursor: pointer;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-radius: 6px;
    padding: 0.4rem 0.8rem;
    font-size: 0.85rem;
    font-weight: 600;
  }
  .upload-trigger:hover {
    background: var(--rw-accent-hover);
  }
  .upload-trigger input[type='file'] {
    display: none;
  }
  .photo-grid {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 10px;
  }
  .photo-grid li {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .photo-thumb {
    background: none;
    border: 1px solid var(--rw-border);
    border-radius: 8px;
    padding: 0;
    cursor: pointer;
    overflow: hidden;
    aspect-ratio: 1 / 1;
  }
  .photo-thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .photo-meta {
    font-size: 0.7rem;
  }
  .photo-delete {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: 0;
    background: rgba(0, 0, 0, 0.55);
    color: #fff;
    font-size: 1rem;
    line-height: 1;
    cursor: pointer;
  }
  .photo-delete:hover {
    background: rgba(220, 38, 38, 0.85);
  }
  .lightbox {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.85);
    border: 0;
    padding: 0;
    cursor: zoom-out;
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .lightbox img {
    max-width: min(90vw, 1200px);
    max-height: 90vh;
    object-fit: contain;
  }
  .tag-list {
    list-style: none;
    padding: 0;
    margin: 8px 0 12px;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .tag-list li {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .tag-chip {
    background: var(--rw-surface-alt);
    border: 1px solid var(--rw-border);
    color: var(--rw-text);
    padding: 4px 10px;
    border-radius: 999px;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .tag-chip:hover:not(:disabled) {
    border-color: var(--rw-accent);
  }
  .tag-chip.voted {
    background: rgba(252, 82, 0, 0.15);
    border-color: var(--rw-accent);
    color: var(--rw-text);
  }
  .tag-chip:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .tag-count {
    font-size: 0.7rem;
    background: rgba(0, 0, 0, 0.06);
    padding: 1px 6px;
    border-radius: 999px;
    color: var(--rw-text-muted);
    font-variant-numeric: tabular-nums;
  }
  .tag-chip.voted .tag-count {
    background: rgba(252, 82, 0, 0.25);
    color: var(--rw-accent);
  }
  .tag-mod {
    border: 0;
    background: none;
    color: var(--rw-text-faint);
    cursor: pointer;
    padding: 0 4px;
    font-size: 1rem;
    line-height: 1;
  }
  .tag-mod:hover:not(:disabled) {
    color: var(--rw-danger);
  }
  .tag-form {
    display: flex;
    gap: 8px;
    margin-top: 6px;
  }
  .tag-form input {
    flex: 1;
    padding: 0.45rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.9rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .tag-form input:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  .tag-form button {
    padding: 0.45rem 0.95rem;
    border-radius: 6px;
    border: 1px solid var(--rw-accent);
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    font-weight: 600;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .tag-form button:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .tag-err {
    margin-top: 8px;
  }
  .consensus-bar {
    display: flex;
    height: 12px;
    border-radius: 6px;
    overflow: hidden;
    background: var(--rw-surface-alt);
    margin: 8px 0 6px;
  }
  .consensus-bar .seg {
    display: block;
    height: 100%;
  }
  .consensus-bar .easy {
    background: #3b82f6;
  }
  .consensus-bar .right {
    background: var(--rw-success);
  }
  .consensus-bar .hard {
    background: var(--rw-danger);
  }
  .consensus-counts {
    margin: 0 0 12px;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    align-items: center;
  }
  .consensus-counts .dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 4px;
    vertical-align: middle;
  }
  .consensus-counts .dot.easy {
    background: #3b82f6;
  }
  .consensus-counts .dot.right {
    background: var(--rw-success);
  }
  .consensus-counts .dot.hard {
    background: var(--rw-danger);
  }
  .vote-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    margin-top: 4px;
  }
  .vote-btn {
    padding: 0.4rem 0.85rem;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
  }
  .vote-btn:hover:not(:disabled) {
    border-color: var(--rw-accent);
  }
  .vote-btn.vote-easy.active {
    background: rgba(59, 130, 246, 0.12);
    border-color: #3b82f6;
    color: #1d4ed8;
  }
  .vote-btn.vote-right.active {
    background: rgba(22, 163, 74, 0.12);
    border-color: var(--rw-success);
    color: #15803d;
  }
  .vote-btn.vote-hard.active {
    background: rgba(239, 68, 68, 0.12);
    border-color: var(--rw-danger);
    color: #b91c1c;
  }
  .vote-btn:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .histogram {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .hist-row {
    display: grid;
    grid-template-columns: 2.5rem 1fr 2rem;
    align-items: center;
    gap: 8px;
    font-size: 0.85rem;
  }
  .hist-label {
    color: var(--rw-text-muted);
    font-weight: 600;
  }
  .hist-bar {
    background: var(--rw-surface-alt);
    border-radius: 4px;
    height: 14px;
    overflow: hidden;
  }
  .hist-fill {
    display: block;
    background: var(--rw-accent);
    height: 100%;
  }
  .hist-count {
    text-align: right;
    color: var(--rw-text-muted);
  }
  .ascent-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .ascent-list li {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 0.5rem 0;
    border-top: 1px solid var(--rw-border);
  }
  .ascent-list li:first-child {
    border-top: none;
  }
  .ascent-type {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 1px 7px;
    border-radius: 4px;
    font-weight: 700;
    align-self: flex-start;
  }
  .ascent-send,
  .ascent-flash {
    background: rgba(22, 163, 74, 0.15);
    color: #15803d;
  }
  .ascent-attempt {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .ascent-meta {
    font-size: 0.8rem;
  }
  .ascent-notes {
    font-size: 0.88rem;
    color: var(--rw-text);
  }
  .climber-actions form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }
  .action-row {
    display: flex;
    gap: 1rem;
    align-items: flex-end;
    flex-wrap: wrap;
  }
  .ascent-type-group {
    border: 0;
    padding: 0;
    margin: 0;
    display: inline-flex;
    background: var(--rw-surface-alt);
    border-radius: 8px;
    overflow: hidden;
  }
  .seg {
    cursor: pointer;
    padding: 0.5rem 0.85rem;
    font-size: 0.85rem;
    font-weight: 600;
    text-transform: capitalize;
    color: var(--rw-text-muted);
  }
  .seg.active {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
  }
  .seg input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .attempts-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--rw-text-muted);
    font-weight: 600;
  }
  .attempts-field input {
    width: 5.5rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
  }
  .notes-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--rw-text-muted);
    font-weight: 600;
  }
  .notes-field textarea {
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-family: inherit;
    box-sizing: border-box;
  }
  .star-group {
    border: 0;
    padding: 0;
    margin: 0;
    display: inline-flex;
    gap: 4px;
  }
  .star {
    cursor: pointer;
    font-size: 1.6rem;
    color: var(--rw-border-strong);
    transition: color 80ms;
  }
  .star.filled {
    color: var(--rw-accent);
  }
  .star input {
    position: absolute;
    opacity: 0;
    pointer-events: none;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  .prior-tick {
    margin: -0.25rem 0 0.25rem;
    font-size: 0.85rem;
  }
  button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
    align-self: flex-start;
    padding: 0.5rem 1rem;
  }
  button.primary:hover:not(:disabled) {
    background: var(--rw-accent-hover);
  }
  .ok {
    background: rgba(22, 163, 74, 0.1);
    border: 1px solid rgba(22, 163, 74, 0.3);
    color: #15803d;
    padding: 0.45rem 0.7rem;
    border-radius: 6px;
    font-size: 0.85rem;
    margin: 0;
  }
  .danger-zone {
    border-color: #fde2e2;
    background: #fffafa;
  }
  .danger-zone h2 {
    color: #991b1b;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  button {
    cursor: pointer;
    padding: 0.45rem 0.85rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.85rem;
    font-weight: 600;
    text-transform: capitalize;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
    text-transform: none;
  }
  button.danger:hover:not(:disabled) {
    background: #fef2f2;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.5rem 0 0;
  }
</style>
