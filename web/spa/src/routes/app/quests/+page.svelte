<script lang="ts">
  import {
    listQuests,
    listMyQuests,
    getLocation,
    ApiClientError,
    type QuestListItemShape,
    type ClimberQuestShape,
    type LocationShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';
  import { goto } from '$app/navigation';

  let catalog = $state<QuestListItemShape[]>([]);
  let myQuests = $state<ClimberQuestShape[]>([]);
  let location = $state<LocationShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let domainFilter = $state('');
  let view = $state<'browse' | 'mine'>('browse');

  const locId = $derived(effectiveLocationId());
  // Defer to the server's progressions_enabled flag — the same one the
  // HTMX side gates on (see progressions_climber.go::progressionsGated).
  // If a user types /app/quests at a location that hasn't enabled the
  // feature, render a "not enabled" panel instead of attempting the
  // catalog fetch (which the server would 403 anyway).
  const enabled = $derived(location ? !!location.progressions_enabled : null);
  // Setter+ at this location can preview the catalog while it's still
  // off, so they can set things up before flipping the feature flag.
  // Climbers who navigate to /app/quests at a disabled location get
  // bounced to the dashboard — no nav link, no empty-state.
  const isStaff = $derived(roleRankAt(locId) >= 2);

  $effect(() => {
    if (enabled === false && !isStaff) {
      goto('/app');
    }
  });

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    // Resolve the location first so we can decide whether to fetch the
    // quest catalog at all. Both fetches in parallel — if the location
    // turns out to have progressions disabled, we'll show a panel and
    // ignore the (possibly empty) catalog response.
    Promise.all([
      getLocation(locId),
      listQuests(locId).catch(() => [] as QuestListItemShape[]),
      listMyQuests().catch(() => [] as ClimberQuestShape[]),
    ])
      .then(([loc, cat, mine]) => {
        if (cancelled) return;
        location = loc;
        catalog = cat;
        myQuests = mine;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load quests.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  // Domain filter dropdown — derived from the catalog so it always matches
  // what's actually available.
  const domains = $derived.by(() => {
    const seen = new Map<string, string>();
    for (const q of catalog) {
      if (q.domain && !seen.has(q.domain.id)) seen.set(q.domain.id, q.domain.name);
    }
    return Array.from(seen.entries()).map(([id, name]) => ({ id, name }));
  });

  const visibleCatalog = $derived.by(() => {
    if (!domainFilter) return catalog;
    return catalog.filter((q) => q.domain_id === domainFilter);
  });

  // Map quest_id → user's enrollment, so the catalog cards can show
  // "active" / "completed" / "abandoned" state.
  const enrollmentByQuest = $derived.by(() => {
    const m = new Map<string, ClimberQuestShape>();
    for (const cq of myQuests) m.set(cq.quest_id, cq);
    return m;
  });

  const activeMine = $derived(myQuests.filter((q) => q.status === 'active'));
  const completedMine = $derived(myQuests.filter((q) => q.status === 'completed'));

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString();
  }
</script>

<svelte:head>
  <title>Quests — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Quests</h1>
      <p class="lede">Pick a challenge, log your progress, earn badges.</p>
    </div>
    {#if enabled}
      <div class="header-actions">
        <div class="view-toggle">
          <button class:active={view === 'browse'} onclick={() => (view = 'browse')}>
            Browse
          </button>
          <button class:active={view === 'mine'} onclick={() => (view = 'mine')}>
            My quests ({myQuests.length})
          </button>
        </div>
        <a class="badges-link" href="/app/quests/badges">Badges →</a>
        <a class="badges-link" href="/app/quests/activity">Activity →</a>
      </div>
    {/if}
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar to see its quest catalog.</p>
  {:else if loading}
    <p class="muted">Loading quests…</p>
  {:else if enabled === false}
    <!-- Climbers redirect away (effect above); only staff land here. -->
    <div class="empty-card">
      <h3>Quests aren't enabled here yet</h3>
      <p>
        Climbers won't see this page until the feature flag is on. Build out
        the quest catalog under
        <a class="link" href="/app/settings/progressions">progressions admin</a>,
        then flip the toggle in <a class="link" href="/app/settings/gym">gym settings</a>.
      </p>
    </div>
  {:else if error}
    <p class="error">{error}</p>
  {:else if view === 'browse'}
    {#if catalog.length === 0}
      <div class="empty-card">
        <h3>No quests yet</h3>
        <p>Ask a head setter to publish a quest catalog at this location.</p>
      </div>
    {:else}
      {#if domains.length > 1}
        <div class="filter-bar">
          <label>
            <span>Domain</span>
            <select bind:value={domainFilter}>
              <option value="">All domains</option>
              {#each domains as d (d.id)}
                <option value={d.id}>{d.name}</option>
              {/each}
            </select>
          </label>
        </div>
      {/if}

      <ul class="quest-grid">
        {#each visibleCatalog as q (q.id)}
          {@const cq = enrollmentByQuest.get(q.id)}
          <li>
            <a class="quest-card" href="/app/quests/{q.id}">
              <div class="quest-head">
                {#if q.domain}
                  <span class="domain-chip" style="background:{q.domain.color ?? '#cbd5e1'}">
                    {q.domain.icon ?? '◆'} {q.domain.name}
                  </span>
                {/if}
                {#if cq}
                  <span class="status-pill status-{cq.status}">{cq.status}</span>
                {/if}
              </div>
              <h3 class="quest-name">{q.name}</h3>
              <p class="quest-desc">{q.description}</p>
              <div class="quest-foot muted">
                {#if q.target_count}
                  <span>Target: {q.target_count}</span>
                  <span>·</span>
                {/if}
                <span>{q.skill_level}</span>
                <span>·</span>
                <span>{q.active_count} active · {q.completed_count} done</span>
              </div>
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  {:else if myQuests.length === 0}
    <div class="empty-card">
      <h3>No quests started</h3>
      <p>Browse the catalog and start one.</p>
      <button class="btn-primary" onclick={() => (view = 'browse')}>Browse quests</button>
    </div>
  {:else}
    {#if activeMine.length > 0}
      <section class="section">
        <h2>Active</h2>
        <ul class="quest-grid">
          {#each activeMine as cq (cq.id)}
            <li>
              <a class="quest-card" href="/app/quests/{cq.quest_id}">
                <div class="quest-head">
                  {#if cq.quest?.domain}
                    <span class="domain-chip" style="background:{cq.quest.domain.color ?? '#cbd5e1'}">
                      {cq.quest.domain.name}
                    </span>
                  {/if}
                  <span class="status-pill status-active">active</span>
                </div>
                <h3 class="quest-name">{cq.quest?.name ?? 'Quest'}</h3>
                <div class="progress">
                  {#if cq.quest?.target_count}
                    <div class="progress-bar">
                      <span
                        class="progress-fill"
                        style="width: {Math.min(100, (cq.progress_count / cq.quest.target_count) * 100)}%"></span>
                    </div>
                    <span class="progress-text muted">
                      {cq.progress_count} / {cq.quest.target_count}
                    </span>
                  {:else}
                    <span class="progress-text muted">{cq.progress_count} logged</span>
                  {/if}
                </div>
                <div class="quest-foot muted">Started {fmtDate(cq.started_at)}</div>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if completedMine.length > 0}
      <section class="section">
        <h2>Completed</h2>
        <ul class="quest-grid">
          {#each completedMine as cq (cq.id)}
            <li>
              <a class="quest-card" href="/app/quests/{cq.quest_id}">
                <div class="quest-head">
                  {#if cq.quest?.domain}
                    <span class="domain-chip" style="background:{cq.quest.domain.color ?? '#cbd5e1'}">
                      {cq.quest.domain.name}
                    </span>
                  {/if}
                  <span class="status-pill status-completed">completed</span>
                </div>
                <h3 class="quest-name">{cq.quest?.name ?? 'Quest'}</h3>
                <div class="quest-foot muted">
                  Completed {cq.completed_at ? fmtDate(cq.completed_at) : ''}
                </div>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 64rem;
  }
  .page-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
    flex-wrap: wrap;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
    letter-spacing: -0.01em;
  }
  h2 {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0 0 0.85rem;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
  }
  .view-toggle {
    display: inline-flex;
    border: 1px solid var(--rw-border-strong);
    border-radius: 8px;
    overflow: hidden;
  }
  .view-toggle button {
    padding: 0.45rem 0.85rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    border: none;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
  }
  .header-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: center;
  }
  .badges-link {
    color: var(--rw-text);
    font-weight: 600;
    font-size: 0.85rem;
    text-decoration: none;
    border: 1px solid var(--rw-border-strong);
    padding: 0.4rem 0.85rem;
    border-radius: 6px;
  }
  .badges-link:hover {
    border-color: var(--rw-accent);
    color: var(--rw-accent);
  }
  .view-toggle button.active {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
  }
  .filter-bar {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 0.85rem 1rem;
    margin-bottom: 1rem;
    display: flex;
    gap: 1rem;
  }
  .filter-bar label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--rw-text-muted);
    font-weight: 600;
    flex: 1 1 12rem;
  }
  .filter-bar select {
    padding: 0.45rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
    background: var(--rw-surface);
  }
  .section {
    margin-bottom: 1.5rem;
  }
  .quest-grid {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    gap: 1rem;
  }
  .quest-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1rem 1.1rem;
    text-decoration: none;
    color: inherit;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    transition: border-color 120ms, transform 120ms;
    height: 100%;
  }
  .quest-card:hover {
    border-color: var(--rw-accent);
    transform: translateY(-1px);
  }
  .quest-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }
  .domain-chip {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    color: #fff;
    font-weight: 700;
    text-shadow: 0 1px 0 rgba(0, 0, 0, 0.15);
  }
  .status-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .status-active {
    background: rgba(245, 158, 11, 0.2);
    color: #92590a;
  }
  .status-completed {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-abandoned {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .quest-name {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0;
    line-height: 1.3;
  }
  .quest-desc {
    color: var(--rw-text-muted);
    font-size: 0.88rem;
    margin: 0;
    line-height: 1.4;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .quest-foot {
    font-size: 0.8rem;
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    margin-top: auto;
  }
  .progress {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .progress-bar {
    flex: 1;
    background: var(--rw-surface-alt);
    border-radius: 4px;
    height: 8px;
    overflow: hidden;
  }
  .progress-fill {
    display: block;
    background: var(--rw-accent);
    height: 100%;
  }
  .progress-text {
    font-size: 0.8rem;
    white-space: nowrap;
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
    margin: 0 0 1.25rem;
  }
  .empty-card .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
  }
  .btn-primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    padding: 0.55rem 1rem;
    border-radius: 8px;
    border: 1px solid var(--rw-accent);
    font-weight: 600;
    font-size: 0.9rem;
    cursor: pointer;
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
