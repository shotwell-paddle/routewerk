<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import {
    listQuestDomains,
    listBadges,
    listOrgTags,
    getQuest,
    updateQuest,
    ApiClientError,
    type QuestDomainShape,
    type BadgeShape,
    type TagShape,
    type QuestShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { authState, roleRankAt } from '$lib/stores/auth.svelte';
  import QuestForm from '$lib/components/QuestForm.svelte';

  const questId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());
  const canAdmin = $derived(roleRankAt(locId) >= 3);

  let quest = $state<QuestShape | null>(null);
  let domains = $state<QuestDomainShape[]>([]);
  let badges = $state<BadgeShape[]>([]);
  let tags = $state<TagShape[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);
  let savedFlash = $state<string | null>(null);

  $effect(() => {
    if (!authState().loaded || !authState().me) return;
    if (!canAdmin) {
      goto('/settings');
    }
  });

  $effect(() => {
    if (!locId || !questId) return;
    let cancelled = false;
    loading = true;
    (async () => {
      try {
        const [q, d, b] = await Promise.all([
          getQuest(questId),
          listQuestDomains(locId).catch(() => [] as QuestDomainShape[]),
          listBadges(locId).catch(() => [] as BadgeShape[]),
        ]);
        if (cancelled) return;
        quest = q;
        domains = d;
        badges = b;

        const me = authState().me;
        const orgID =
          me?.memberships.find((m) => m.location_id === locId)?.org_id ??
          me?.memberships[0]?.org_id;
        if (orgID) {
          try {
            tags = await listOrgTags(orgID);
          } catch {
            // Best-effort.
          }
        }
      } catch (err) {
        if (!cancelled) {
          error = err instanceof ApiClientError ? err.message : 'Could not load quest.';
        }
      } finally {
        if (!cancelled) loading = false;
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  async function submit(body: Partial<QuestShape>) {
    if (!questId) return;
    saving = true;
    error = null;
    savedFlash = null;
    try {
      quest = await updateQuest(questId, body);
      savedFlash = 'Saved.';
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not update quest.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>{quest ? `Edit ${quest.name}` : 'Edit quest'} — Routewerk</title>
</svelte:head>

<div class="page">
  <p><a class="back" href="/settings/progressions">← Progressions admin</a></p>
  <h1>{quest ? `Edit "${quest.name}"` : 'Edit quest'}</h1>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar first.</p>
  {:else if !canAdmin}
    <p class="muted">Only head setters and above can edit quests.</p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else if !quest}
    <p class="error">{error ?? 'Quest not found.'}</p>
  {:else}
    {#if savedFlash}<p class="ok">{savedFlash}</p>{/if}
    <QuestForm
      initial={quest}
      {domains}
      {badges}
      {tags}
      submitLabel="Save quest"
      onSubmit={submit}
      onCancel={() => goto('/settings/progressions')}
      {saving}
      {error} />
  {/if}
</div>

<style>
  .page {
    max-width: 48rem;
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
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0.25rem 0 1.25rem;
    letter-spacing: -0.01em;
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
  .ok {
    background: rgba(22, 163, 74, 0.1);
    border: 1px solid rgba(22, 163, 74, 0.3);
    color: #15803d;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0 0 1rem;
  }
</style>
