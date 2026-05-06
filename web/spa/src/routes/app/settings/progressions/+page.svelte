<script lang="ts">
  import {
    listAllQuests,
    deactivateQuest,
    duplicateQuest,
    listQuestDomains,
    createQuestDomain,
    deleteQuestDomain,
    listBadges,
    createBadge,
    deleteBadge,
    ApiClientError,
    type QuestListItemShape,
    type QuestDomainShapeFull,
    type BadgeShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';

  let quests = $state<QuestListItemShape[]>([]);
  let domains = $state<QuestDomainShapeFull[]>([]);
  let badges = $state<BadgeShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let mutating = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());
  const canAdmin = $derived(roleRankAt(locId) >= 3);

  async function refresh() {
    if (!locId) return;
    const [q, d, b] = await Promise.all([
      listAllQuests(locId).catch(() => [] as QuestListItemShape[]),
      listQuestDomains(locId).catch(() => [] as QuestDomainShapeFull[]),
      listBadges(locId).catch(() => [] as BadgeShape[]),
    ]);
    quests = q;
    domains = d;
    badges = b;
  }

  $effect(() => {
    if (!locId || !canAdmin) {
      loading = false;
      return;
    }
    let cancelled = false;
    loading = true;
    error = null;
    refresh()
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load progressions.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  // ── Quest actions ────────────────────────────────────────

  async function deactivate(q: QuestListItemShape) {
    if (!locId) return;
    if (!confirm(`Deactivate "${q.name}"? Existing enrollments stay; new climbers can't start it.`)) return;
    mutating = q.id;
    try {
      await deactivateQuest(locId, q.id);
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Deactivate failed.';
    } finally {
      mutating = null;
    }
  }

  async function duplicate(q: QuestListItemShape) {
    if (!locId) return;
    mutating = q.id;
    try {
      await duplicateQuest(locId, q.id);
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Duplicate failed.';
    } finally {
      mutating = null;
    }
  }

  // ── Domain create form ────────────────────────────────────

  let newDomain = $state({ name: '', color: '#fc5200', icon: '◆' });
  let domainSaving = $state(false);

  async function addDomain() {
    if (!locId) return;
    if (!newDomain.name.trim()) return;
    domainSaving = true;
    try {
      await createQuestDomain(locId, {
        name: newDomain.name.trim(),
        color: newDomain.color,
        icon: newDomain.icon || '◆',
        sort_order: domains.length,
      });
      newDomain = { name: '', color: '#fc5200', icon: '◆' };
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not create domain.';
    } finally {
      domainSaving = false;
    }
  }

  async function removeDomain(d: QuestDomainShapeFull) {
    if (!locId) return;
    if (!confirm(`Delete domain "${d.name}"? Quests in this domain must be moved or deleted first.`)) return;
    mutating = d.id;
    try {
      await deleteQuestDomain(locId, d.id);
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not delete domain.';
    } finally {
      mutating = null;
    }
  }

  // ── Badge create form ────────────────────────────────────

  let newBadge = $state({ name: '', icon: '🏅', color: '#fc5200' });
  let badgeSaving = $state(false);

  async function addBadge() {
    if (!locId) return;
    if (!newBadge.name.trim()) return;
    badgeSaving = true;
    try {
      await createBadge(locId, {
        name: newBadge.name.trim(),
        icon: newBadge.icon || '🏅',
        color: newBadge.color,
      });
      newBadge = { name: '', icon: '🏅', color: '#fc5200' };
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not create badge.';
    } finally {
      badgeSaving = false;
    }
  }

  async function removeBadge(b: BadgeShape) {
    if (!confirm(`Delete badge "${b.name}"?`)) return;
    mutating = b.id;
    try {
      await deleteBadge(b.id);
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not delete badge.';
    } finally {
      mutating = null;
    }
  }
</script>

<svelte:head>
  <title>Progressions admin — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/settings">← Settings</a>
  <h1>Progressions admin</h1>
  <p class="lede">Quest catalog, domains, and badges for this location.</p>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if !canAdmin}
    <p class="muted">Head setter or above required to admin progressions.</p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else}
    {#if error}<p class="error">{error}</p>{/if}

    <section class="card">
      <h2>Quests ({quests.length})</h2>
      {#if quests.length === 0}
        <p class="muted">No quests yet. Build the domains + badges below first, then create quests in the existing HTMX editor at <a class="link" href="/settings/progressions/quests/new">/settings/progressions/quests/new</a>.</p>
      {:else}
        <ul class="list">
          {#each quests as q (q.id)}
            <li>
              <div class="row">
                <div>
                  <span class="name">{q.name}</span>
                  {#if !q.is_active}<span class="pill pill-inactive">inactive</span>{/if}
                  {#if q.domain}<span class="pill" style="background:{q.domain.color ?? '#cbd5e1'}; color:#fff">{q.domain.name}</span>{/if}
                </div>
                <div class="actions">
                  <a class="link" href="/settings/progressions/quests/{q.id}/edit">Edit</a>
                  <button onclick={() => duplicate(q)} disabled={mutating === q.id}>Duplicate</button>
                  {#if q.is_active}
                    <button class="danger" onclick={() => deactivate(q)} disabled={mutating === q.id}>Deactivate</button>
                  {/if}
                </div>
              </div>
              <p class="qmeta muted small">
                {q.skill_level}
                {#if q.target_count}· target {q.target_count}{/if}
                · {q.active_count} active · {q.completed_count} completed
              </p>
            </li>
          {/each}
        </ul>
      {/if}
      <p class="hint muted small">
        Create + edit quests via the existing HTMX editor for now (the SPA list is read-only).
        <a class="link" href="/settings/progressions/quests/new">+ Create quest</a>
      </p>
    </section>

    <section class="card">
      <h2>Domains ({domains.length})</h2>
      <p class="muted small">High-level groupings (Power, Endurance, Technique, etc).</p>
      {#if domains.length === 0}
        <p class="muted">No domains yet — add at least one before creating quests.</p>
      {:else}
        <ul class="list">
          {#each domains as d (d.id)}
            <li>
              <div class="row">
                <div>
                  <span class="domain-chip" style="background:{d.color ?? '#cbd5e1'}">{d.icon ?? '◆'} {d.name}</span>
                </div>
                <button class="danger" disabled={mutating === d.id} onclick={() => removeDomain(d)}>Delete</button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
      <form class="add-form" onsubmit={(e) => { e.preventDefault(); addDomain(); }}>
        <input bind:value={newDomain.name} placeholder="Domain name (e.g. Power)" />
        <input bind:value={newDomain.icon} placeholder="◆" maxlength="3" class="icon-field" />
        <input type="color" bind:value={newDomain.color} />
        <button class="primary" type="submit" disabled={domainSaving || !newDomain.name.trim()}>
          {domainSaving ? '…' : 'Add'}
        </button>
      </form>
    </section>

    <section class="card">
      <h2>Badges ({badges.length})</h2>
      <p class="muted small">Awarded on quest completion.</p>
      {#if badges.length === 0}
        <p class="muted">No badges yet.</p>
      {:else}
        <ul class="list">
          {#each badges as b (b.id)}
            <li>
              <div class="row">
                <div>
                  <span class="badge-chip" style="background:{b.color}">{b.icon} {b.name}</span>
                </div>
                <button class="danger" disabled={mutating === b.id} onclick={() => removeBadge(b)}>Delete</button>
              </div>
              {#if b.description}<p class="muted small">{b.description}</p>{/if}
            </li>
          {/each}
        </ul>
      {/if}
      <form class="add-form" onsubmit={(e) => { e.preventDefault(); addBadge(); }}>
        <input bind:value={newBadge.name} placeholder="Badge name" />
        <input bind:value={newBadge.icon} placeholder="🏅" maxlength="3" class="icon-field" />
        <input type="color" bind:value={newBadge.color} />
        <button class="primary" type="submit" disabled={badgeSaving || !newBadge.name.trim()}>
          {badgeSaving ? '…' : 'Add'}
        </button>
      </form>
    </section>
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
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
    letter-spacing: -0.01em;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0 0 1.25rem;
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
    margin: 0 0 0.5rem;
  }
  .small {
    font-size: 0.85rem;
  }
  .list {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .list li {
    border-top: 1px solid var(--rw-border);
    padding-top: 8px;
  }
  .list li:first-child {
    border-top: none;
    padding-top: 0;
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }
  .name {
    font-weight: 600;
  }
  .pill {
    display: inline-block;
    margin-left: 6px;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .pill-inactive {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .domain-chip,
  .badge-chip {
    display: inline-block;
    color: #fff;
    padding: 4px 10px;
    border-radius: 4px;
    font-size: 0.85rem;
    font-weight: 700;
    text-shadow: 0 1px 0 rgba(0, 0, 0, 0.15);
  }
  .qmeta {
    margin: 4px 0 0;
  }
  .actions {
    display: inline-flex;
    gap: 6px;
    align-items: center;
  }
  .add-form {
    display: flex;
    gap: 8px;
    align-items: center;
    border-top: 1px dashed var(--rw-border);
    padding-top: 0.75rem;
  }
  .add-form input:not([type='color']):not(.icon-field) {
    flex: 1;
    padding: 0.45rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.9rem;
  }
  .icon-field {
    width: 3.5rem;
    text-align: center;
    padding: 0.45rem 0.5rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
  }
  .add-form input[type='color'] {
    width: 36px;
    height: 32px;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    padding: 0;
    cursor: pointer;
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
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
  }
  button.primary:hover:not(:disabled) {
    background: var(--rw-accent-hover);
  }
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
  }
  button.danger:hover:not(:disabled) {
    background: #fef2f2;
  }
  .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
    font-size: 0.85rem;
    font-weight: 600;
  }
  .hint {
    margin: 0.5rem 0 0;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0 0 0.75rem;
  }
</style>
