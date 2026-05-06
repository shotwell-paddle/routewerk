<script lang="ts">
  import { goto } from '$app/navigation';
  import {
    listQuestDomains,
    listBadges,
    createQuest,
    listOrgLocations,
    listOrgTags,
    ApiClientError,
    type QuestDomainShape,
    type BadgeShape,
    type TagShape,
    type QuestShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { authState, roleRankAt } from '$lib/stores/auth.svelte';
  import QuestForm from '$lib/components/QuestForm.svelte';

  // Backed by the same JSON endpoints used elsewhere (#75). The new
  // form here lets head_setter+ create a quest without leaving the SPA;
  // up to now /settings/progressions linked out to HTMX for create.

  const locId = $derived(effectiveLocationId());
  const canAdmin = $derived(roleRankAt(locId) >= 3);

  let domains = $state<QuestDomainShape[]>([]);
  let badges = $state<BadgeShape[]>([]);
  let tags = $state<TagShape[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let error = $state<string | null>(null);

  // Page-level role gate. Anyone below head_setter bounces.
  $effect(() => {
    if (!authState().loaded || !authState().me) return;
    if (!canAdmin) {
      goto('/settings');
    }
  });

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    (async () => {
      try {
        const [d, b] = await Promise.all([
          listQuestDomains(locId).catch(() => [] as QuestDomainShape[]),
          listBadges(locId).catch(() => [] as BadgeShape[]),
        ]);
        if (cancelled) return;
        domains = d;
        badges = b;

        // Tags are org-scoped, not location-scoped, so we resolve the
        // org via the user's membership row.
        const me = authState().me;
        const orgID =
          me?.memberships.find((m) => m.location_id === locId)?.org_id ??
          me?.memberships[0]?.org_id;
        if (orgID) {
          try {
            tags = await listOrgTags(orgID);
          } catch {
            // Best-effort; the form falls back to a free-text input.
          }
        }
      } catch (err) {
        if (!cancelled) {
          error = err instanceof ApiClientError ? err.message : 'Could not load form data.';
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
    if (!locId) return;
    saving = true;
    error = null;
    try {
      const created = await createQuest(locId, body);
      goto(`/settings/progressions/quests/${created.id}/edit`);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not create quest.';
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>New quest — Routewerk</title>
</svelte:head>

<div class="page">
  <p><a class="back" href="/settings/progressions">← Progressions admin</a></p>
  <h1>New quest</h1>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar first.</p>
  {:else if !canAdmin}
    <p class="muted">Only head setters and above can create quests.</p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else if domains.length === 0}
    <p class="muted">
      Create at least one quest domain first under
      <a class="link" href="/settings/progressions">progressions admin</a>.
    </p>
  {:else}
    <QuestForm
      {domains}
      {badges}
      {tags}
      submitLabel="Create quest"
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
  .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
  }
</style>
