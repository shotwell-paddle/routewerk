<script lang="ts">
  import type {
    QuestShape,
    QuestDomainShape,
    BadgeShape,
    TagShape,
  } from '$lib/api/client';

  // Reusable quest create/edit form. Mirrors the HTMX setter form at
  // web/templates/setter/quest-form.html so the field set + validation
  // semantics match exactly. The caller supplies dropdown sources (the
  // page wrapper fetches them so we don't double-fetch on every mount).

  let {
    initial,
    domains,
    badges,
    tags = [],
    submitLabel,
    onSubmit,
    onCancel,
    saving = false,
    error = null,
  }: {
    initial?: QuestShape | null;
    domains: QuestDomainShape[];
    badges: BadgeShape[];
    tags?: TagShape[];
    submitLabel: string;
    onSubmit: (body: Partial<QuestShape>) => void | Promise<void>;
    onCancel?: () => void;
    saving?: boolean;
    error?: string | null;
  } = $props();

  // svelte-ignore state_referenced_locally
  const seed = initial;

  // Quest types map to scoring rules (see internal/service/quest_service.go).
  // Names align 1:1 with the HTMX form options.
  const QUEST_TYPES = [
    { value: 'route_count', label: 'Route count' },
    { value: 'grade_pyramid', label: 'Grade pyramid' },
    { value: 'tag_collection', label: 'Tag collection' },
    { value: 'session_count', label: 'Session count' },
    { value: 'manual', label: 'Manual (admin marks complete)' },
  ];

  const SKILL_LEVELS = [
    { value: 'beginner', label: 'Beginner' },
    { value: 'intermediate', label: 'Intermediate' },
    { value: 'advanced', label: 'Advanced' },
    { value: 'all', label: 'Any level' },
  ];

  function isoDate(input: string | null | undefined): string {
    if (!input) return '';
    return input.slice(0, 10);
  }

  let form = $state({
    name: seed?.name ?? '',
    description: seed?.description ?? '',
    domain_id: seed?.domain_id ?? '',
    badge_id: seed?.badge_id ?? '',
    quest_type: seed?.quest_type ?? 'route_count',
    completion_criteria: seed?.completion_criteria ?? '',
    target_count: seed?.target_count != null ? String(seed.target_count) : '',
    suggested_duration_days:
      seed?.suggested_duration_days != null ? String(seed.suggested_duration_days) : '',
    available_from: isoDate(seed?.available_from ?? null),
    available_until: isoDate(seed?.available_until ?? null),
    skill_level: seed?.skill_level ?? 'all',
    requires_certification: seed?.requires_certification ?? '',
    route_tag_filter: (seed?.route_tag_filter ?? []).join(','),
    is_active: seed?.is_active ?? true,
    sort_order: seed?.sort_order != null ? String(seed.sort_order) : '0',
  });

  let localError = $state<string | null>(null);

  // Some quest types use target_count, some don't. The HTMX form hides
  // the field when irrelevant; do the same here so users don't enter
  // values that the server will ignore.
  const usesTargetCount = $derived(
    form.quest_type === 'route_count' ||
      form.quest_type === 'session_count' ||
      form.quest_type === 'tag_collection',
  );
  const usesTagFilter = $derived(form.quest_type === 'tag_collection');

  function buildBody(): Partial<QuestShape> | null {
    if (!form.name.trim()) {
      localError = 'Name is required.';
      return null;
    }
    if (!form.domain_id) {
      localError = 'Domain is required.';
      return null;
    }

    const body: Partial<QuestShape> = {
      name: form.name.trim(),
      description: form.description.trim(),
      domain_id: form.domain_id,
      badge_id: form.badge_id || null,
      quest_type: form.quest_type,
      completion_criteria: form.completion_criteria.trim(),
      skill_level: form.skill_level,
      requires_certification: form.requires_certification.trim() || null,
      is_active: form.is_active,
      sort_order: parseInt(form.sort_order, 10) || 0,
    };

    if (usesTargetCount && form.target_count.trim()) {
      const n = parseInt(form.target_count, 10);
      if (isNaN(n) || n <= 0) {
        localError = 'Target count must be a positive number.';
        return null;
      }
      body.target_count = n;
    } else {
      body.target_count = null;
    }

    if (form.suggested_duration_days.trim()) {
      const n = parseInt(form.suggested_duration_days, 10);
      if (isNaN(n) || n <= 0) {
        localError = 'Suggested duration must be a positive number.';
        return null;
      }
      body.suggested_duration_days = n;
    } else {
      body.suggested_duration_days = null;
    }

    body.available_from = form.available_from ? form.available_from : null;
    body.available_until = form.available_until ? form.available_until : null;
    if (
      body.available_from &&
      body.available_until &&
      body.available_until < body.available_from
    ) {
      localError = 'Available until must be after available from.';
      return null;
    }

    body.route_tag_filter = usesTagFilter
      ? form.route_tag_filter
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
      : [];

    return body;
  }

  function handleSubmit(e: Event) {
    e.preventDefault();
    localError = null;
    const body = buildBody();
    if (!body) return;
    onSubmit(body);
  }
</script>

<form class="form" onsubmit={handleSubmit}>
  <div class="row">
    <div class="field">
      <label for="q-name">Name *</label>
      <input id="q-name" bind:value={form.name} maxlength="120" required />
    </div>
    <div class="field">
      <label for="q-domain">Domain *</label>
      <select id="q-domain" bind:value={form.domain_id} required>
        <option value="">Pick a domain…</option>
        {#each domains as d (d.id)}
          <option value={d.id}>{d.name}</option>
        {/each}
      </select>
    </div>
  </div>

  <div class="field">
    <label for="q-desc">Description</label>
    <textarea id="q-desc" bind:value={form.description} rows="3" placeholder="What climbers are working toward."></textarea>
  </div>

  <div class="row">
    <div class="field">
      <label for="q-type">Quest type *</label>
      <select id="q-type" bind:value={form.quest_type}>
        {#each QUEST_TYPES as t (t.value)}
          <option value={t.value}>{t.label}</option>
        {/each}
      </select>
    </div>
    <div class="field">
      <label for="q-skill">Skill level</label>
      <select id="q-skill" bind:value={form.skill_level}>
        {#each SKILL_LEVELS as s (s.value)}
          <option value={s.value}>{s.label}</option>
        {/each}
      </select>
    </div>
  </div>

  <div class="field">
    <label for="q-criteria">Completion criteria</label>
    <textarea
      id="q-criteria"
      bind:value={form.completion_criteria}
      rows="2"
      placeholder="Human-readable summary the climber sees on the quest card."></textarea>
  </div>

  {#if usesTargetCount}
    <div class="row">
      <div class="field">
        <label for="q-target">Target count</label>
        <input id="q-target" type="number" min="1" step="1" bind:value={form.target_count} />
      </div>
      <div class="field">
        <label for="q-duration">Suggested duration (days)</label>
        <input id="q-duration" type="number" min="1" step="1" bind:value={form.suggested_duration_days} />
      </div>
    </div>
  {/if}

  {#if usesTagFilter}
    <div class="field">
      <label for="q-tags">Route tag filter</label>
      <input
        id="q-tags"
        bind:value={form.route_tag_filter}
        placeholder={tags.length > 0
          ? 'comma-separated, e.g. ' + tags.slice(0, 3).map((t) => t.name).join(', ')
          : 'comma-separated tag names'} />
      {#if tags.length > 0}
        <p class="hint">
          Available: {tags.map((t) => t.name).join(', ')}
        </p>
      {/if}
    </div>
  {/if}

  <div class="row">
    <div class="field">
      <label for="q-from">Available from</label>
      <input id="q-from" type="date" bind:value={form.available_from} />
    </div>
    <div class="field">
      <label for="q-until">Available until</label>
      <input id="q-until" type="date" bind:value={form.available_until} />
    </div>
  </div>

  <div class="row">
    <div class="field">
      <label for="q-badge">Badge awarded on completion</label>
      <select id="q-badge" bind:value={form.badge_id}>
        <option value="">— none —</option>
        {#each badges as b (b.id)}
          <option value={b.id}>{b.name}</option>
        {/each}
      </select>
    </div>
    <div class="field">
      <label for="q-sort">Sort order</label>
      <input id="q-sort" type="number" step="1" bind:value={form.sort_order} />
    </div>
  </div>

  <div class="field">
    <label for="q-cert">Required certification</label>
    <input
      id="q-cert"
      bind:value={form.requires_certification}
      placeholder="e.g. lead-cert (climbers without this won't see the quest)" />
  </div>

  <label class="check">
    <input type="checkbox" bind:checked={form.is_active} />
    Active (climbers can see + start this quest)
  </label>

  {#if localError || error}
    <p class="error">{localError ?? error}</p>
  {/if}

  <div class="actions">
    <button type="submit" class="primary" disabled={saving}>
      {saving ? 'Saving…' : submitLabel}
    </button>
    {#if onCancel}
      <button type="button" onclick={onCancel} disabled={saving}>Cancel</button>
    {/if}
  </div>
</form>

<style>
  .form {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    max-width: 48rem;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.85rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field label {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--rw-text-muted);
  }
  .field input,
  .field select,
  .field textarea {
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    box-sizing: border-box;
    font-family: inherit;
  }
  .field input:focus,
  .field select:focus,
  .field textarea:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  .hint {
    margin: 0;
    font-size: 0.78rem;
    color: var(--rw-text-faint);
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 0.95rem;
  }
  .actions {
    display: flex;
    gap: 8px;
    margin-top: 0.25rem;
  }
  button {
    cursor: pointer;
    padding: 0.55rem 1rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.9rem;
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
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0;
  }
</style>
