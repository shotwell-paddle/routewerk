<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    updateCompetition,
    listEvents,
    createEvent,
    updateEvent,
    listCategories,
    createCategory,
    type Competition,
    type CompetitionEvent,
    type CompetitionCategory,
    type CompetitionFormat,
    type CompetitionStatus,
    type CompetitionUpdate,
    type EventCreate,
    type EventUpdate,
    type CategoryCreate,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  const SCORERS = ['top_zone', 'fixed', 'decay'] as const;
  const STATUSES: CompetitionStatus[] = [
    'draft',
    'open',
    'live',
    'closed',
    'archived',
  ];
  const VISIBILITIES = ['public', 'members', 'registrants'] as const;

  let comp = $state<Competition | null>(null);
  let events = $state<CompetitionEvent[]>([]);
  let categories = $state<CompetitionCategory[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Inline edit modes — only one open at a time keeps the UI focused.
  let compEditing = $state(false);
  let eventCreating = $state(false);
  let editingEventId = $state<string | null>(null);
  let categoryCreating = $state(false);

  const id = $derived(page.params.id ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/app/competitions/' + id));
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;
    try {
      await loadAll();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load competition.';
    } finally {
      loading = false;
    }
  });

  async function loadAll() {
    const [c, evs, cats] = await Promise.all([
      getCompetition(id),
      listEvents(id),
      listCategories(id),
    ]);
    comp = c;
    events = evs;
    categories = cats;
  }

  // ── Comp edit form state ────────────────────────────────

  let compForm = $state({
    name: '',
    slug: '',
    format: 'single' as CompetitionFormat,
    scoring_rule: 'top_zone',
    status: 'draft' as CompetitionStatus,
    leaderboard_visibility: 'public' as 'public' | 'members' | 'registrants',
    starts_at: '',
    ends_at: '',
    registration_opens_at: '',
    registration_closes_at: '',
  });
  let compFormError = $state<string | null>(null);
  let compSaving = $state(false);

  function openCompEdit() {
    if (!comp) return;
    compForm = {
      name: comp.name,
      slug: comp.slug,
      format: comp.format,
      scoring_rule: comp.scoring_rule,
      status: comp.status,
      leaderboard_visibility: comp.leaderboard_visibility,
      starts_at: isoToLocal(comp.starts_at),
      ends_at: isoToLocal(comp.ends_at),
      registration_opens_at: isoToLocal(comp.registration_opens_at),
      registration_closes_at: isoToLocal(comp.registration_closes_at),
    };
    compFormError = null;
    compEditing = true;
  }

  async function saveComp(e: Event) {
    e.preventDefault();
    if (!comp || compSaving) return;
    compFormError = null;
    const startsISO = localToISO(compForm.starts_at);
    const endsISO = localToISO(compForm.ends_at);
    if (!startsISO || !endsISO) {
      compFormError = 'Start and end are required.';
      return;
    }
    if (new Date(endsISO) <= new Date(startsISO)) {
      compFormError = 'End must be after start.';
      return;
    }
    const body: CompetitionUpdate = {
      name: compForm.name,
      slug: compForm.slug,
      format: compForm.format,
      scoring_rule: compForm.scoring_rule,
      status: compForm.status,
      leaderboard_visibility: compForm.leaderboard_visibility,
      starts_at: startsISO,
      ends_at: endsISO,
    };
    const opens = localToISO(compForm.registration_opens_at);
    const closes = localToISO(compForm.registration_closes_at);
    body.registration_opens_at = opens;
    body.registration_closes_at = closes;

    compSaving = true;
    try {
      comp = await updateCompetition(comp.id, body);
      compEditing = false;
    } catch (err) {
      compFormError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      compSaving = false;
    }
  }

  // ── Event create form state ─────────────────────────────

  let evCreate = $state({
    name: '',
    sequence: 1,
    starts_at: '',
    ends_at: '',
    weight: 1,
  });
  let evCreateError = $state<string | null>(null);
  let evCreating = $state(false);

  function openEventCreate() {
    evCreate = {
      name: '',
      sequence: events.length + 1,
      starts_at: comp ? isoToLocal(comp.starts_at) : '',
      ends_at: comp ? isoToLocal(comp.ends_at) : '',
      weight: 1,
    };
    evCreateError = null;
    eventCreating = true;
  }

  async function submitEventCreate(e: Event) {
    e.preventDefault();
    if (!comp || evCreating) return;
    evCreateError = null;
    const startsISO = localToISO(evCreate.starts_at);
    const endsISO = localToISO(evCreate.ends_at);
    if (!evCreate.name || !startsISO || !endsISO) {
      evCreateError = 'Name + dates are required.';
      return;
    }
    if (new Date(endsISO) <= new Date(startsISO)) {
      evCreateError = 'End must be after start.';
      return;
    }
    const body: EventCreate = {
      name: evCreate.name,
      sequence: evCreate.sequence,
      starts_at: startsISO,
      ends_at: endsISO,
      weight: evCreate.weight,
    };
    evCreating = true;
    try {
      const created = await createEvent(comp.id, body);
      events = [...events, created].sort((a, b) => a.sequence - b.sequence);
      eventCreating = false;
    } catch (err) {
      evCreateError = err instanceof ApiClientError ? err.message : 'Create failed.';
    } finally {
      evCreating = false;
    }
  }

  // ── Event edit form state ───────────────────────────────

  let evEdit = $state({
    name: '',
    sequence: 1,
    starts_at: '',
    ends_at: '',
    weight: 1,
  });
  let evEditError = $state<string | null>(null);
  let evSaving = $state(false);

  function openEventEdit(ev: CompetitionEvent) {
    evEdit = {
      name: ev.name,
      sequence: ev.sequence,
      starts_at: isoToLocal(ev.starts_at),
      ends_at: isoToLocal(ev.ends_at),
      weight: ev.weight,
    };
    evEditError = null;
    editingEventId = ev.id;
  }

  async function submitEventEdit(e: Event) {
    e.preventDefault();
    if (!editingEventId || evSaving) return;
    evEditError = null;
    const startsISO = localToISO(evEdit.starts_at);
    const endsISO = localToISO(evEdit.ends_at);
    if (!startsISO || !endsISO) {
      evEditError = 'Dates are required.';
      return;
    }
    const body: EventUpdate = {
      name: evEdit.name,
      sequence: evEdit.sequence,
      starts_at: startsISO,
      ends_at: endsISO,
      weight: evEdit.weight,
    };
    evSaving = true;
    try {
      const updated = await updateEvent(editingEventId, body);
      events = events
        .map((x) => (x.id === updated.id ? updated : x))
        .sort((a, b) => a.sequence - b.sequence);
      editingEventId = null;
    } catch (err) {
      evEditError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      evSaving = false;
    }
  }

  // ── Category create form state ──────────────────────────

  let catCreate = $state({ name: '', sort_order: 0 });
  let catCreateError = $state<string | null>(null);
  let catCreating = $state(false);

  function openCategoryCreate() {
    catCreate = { name: '', sort_order: categories.length };
    catCreateError = null;
    categoryCreating = true;
  }

  async function submitCategoryCreate(e: Event) {
    e.preventDefault();
    if (!comp || catCreating) return;
    if (!catCreate.name) {
      catCreateError = 'Name is required.';
      return;
    }
    const body: CategoryCreate = {
      name: catCreate.name,
      sort_order: catCreate.sort_order,
    };
    catCreating = true;
    try {
      const created = await createCategory(comp.id, body);
      categories = [...categories, created].sort(
        (a, b) => a.sort_order - b.sort_order,
      );
      categoryCreating = false;
    } catch (err) {
      catCreateError = err instanceof ApiClientError ? err.message : 'Create failed.';
    } finally {
      catCreating = false;
    }
  }

  // ── Helpers ─────────────────────────────────────────────

  function fmtDateTime(iso: string): string {
    return new Date(iso).toLocaleString();
  }
  function localToISO(v: string): string | null {
    if (!v) return null;
    const d = new Date(v);
    if (isNaN(d.getTime())) return null;
    return d.toISOString();
  }
  function isoToLocal(iso: string | null | undefined): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    const pad = (n: number) => String(n).padStart(2, '0');
    return (
      d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) +
      'T' + pad(d.getHours()) + ':' + pad(d.getMinutes())
    );
  }
</script>

<svelte:head>
  <title>{comp?.name ?? 'Competition'} — Routewerk staff</title>
</svelte:head>

<main>
  <p><a href="/app/competitions">← Back to competitions</a></p>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if comp}
    <header>
      <h1>{comp.name}</h1>
      <p class="meta">
        <span class="status" data-status={comp.status}>{comp.status}</span>
        · slug: <code>{comp.slug}</code>
        · scoring: <code>{comp.scoring_rule}</code>
        · format: <code>{comp.format}</code>
      </p>
      <p class="meta muted">
        {fmtDateTime(comp.starts_at)} → {fmtDateTime(comp.ends_at)}
      </p>
      <p class="quick-links">
        <a href={`/app/competitions/${comp.id}/problems`}>Problems editor →</a>
        <a href={`/app/competitions/${comp.id}/registrations`}>Registrations →</a>
        <a href={`/comp/${comp.id}`}>Climber view →</a>
        <a href={`/comp/${comp.id}/leaderboard`}>Leaderboard →</a>
      </p>
    </header>

    <!-- ── Comp metadata ──────────────────────────────────── -->
    <section class="card">
      <div class="card-head">
        <h2>Competition details</h2>
        {#if !compEditing}
          <button type="button" class="btn-link" onclick={openCompEdit}>Edit</button>
        {/if}
      </div>

      {#if compEditing}
        <form onsubmit={saveComp}>
          <div class="field">
            <label for="comp-name">Name</label>
            <input id="comp-name" type="text" bind:value={compForm.name} required />
          </div>
          <div class="field">
            <label for="comp-slug">Slug</label>
            <input id="comp-slug" type="text" bind:value={compForm.slug} required pattern="[a-z0-9][a-z0-9-]*" />
          </div>
          <div class="field-row">
            <div class="field">
              <label for="comp-format">Format</label>
              <select id="comp-format" bind:value={compForm.format}>
                <option value="single">Single event</option>
                <option value="series">Series</option>
              </select>
            </div>
            <div class="field">
              <label for="comp-scoring">Scoring rule</label>
              <select id="comp-scoring" bind:value={compForm.scoring_rule}>
                {#each SCORERS as s (s)}<option value={s}>{s}</option>{/each}
              </select>
            </div>
          </div>
          <div class="field-row">
            <div class="field">
              <label for="comp-status">Status</label>
              <select id="comp-status" bind:value={compForm.status}>
                {#each STATUSES as s (s)}<option value={s}>{s}</option>{/each}
              </select>
            </div>
            <div class="field">
              <label for="comp-vis">Leaderboard visibility</label>
              <select id="comp-vis" bind:value={compForm.leaderboard_visibility}>
                {#each VISIBILITIES as v (v)}<option value={v}>{v}</option>{/each}
              </select>
            </div>
          </div>
          <div class="field-row">
            <div class="field">
              <label for="comp-starts">Starts at</label>
              <input id="comp-starts" type="datetime-local" bind:value={compForm.starts_at} required />
            </div>
            <div class="field">
              <label for="comp-ends">Ends at</label>
              <input id="comp-ends" type="datetime-local" bind:value={compForm.ends_at} required />
            </div>
          </div>
          <div class="field-row">
            <div class="field">
              <label for="comp-reg-opens">Reg opens</label>
              <input id="comp-reg-opens" type="datetime-local" bind:value={compForm.registration_opens_at} />
            </div>
            <div class="field">
              <label for="comp-reg-closes">Reg closes</label>
              <input id="comp-reg-closes" type="datetime-local" bind:value={compForm.registration_closes_at} />
            </div>
          </div>
          {#if compFormError}<p class="form-error">{compFormError}</p>{/if}
          <div class="form-actions">
            <button type="submit" class="btn-primary" disabled={compSaving}>
              {compSaving ? 'Saving…' : 'Save'}
            </button>
            <button type="button" class="btn-secondary" onclick={() => (compEditing = false)}>Cancel</button>
          </div>
        </form>
      {:else}
        <dl class="kv">
          <dt>Status</dt>
          <dd><span class="status" data-status={comp.status}>{comp.status}</span></dd>
          <dt>Leaderboard</dt>
          <dd>{comp.leaderboard_visibility}</dd>
          <dt>Registration window</dt>
          <dd>
            {comp.registration_opens_at ? fmtDateTime(comp.registration_opens_at) : '—'}
            →
            {comp.registration_closes_at ? fmtDateTime(comp.registration_closes_at) : '—'}
          </dd>
        </dl>
      {/if}
    </section>

    <!-- ── Events ─────────────────────────────────────────── -->
    <section class="card">
      <div class="card-head">
        <h2>Events ({events.length})</h2>
        {#if !eventCreating}
          <button type="button" class="btn-link" onclick={openEventCreate}>+ Add event</button>
        {/if}
      </div>

      {#if eventCreating}
        <form onsubmit={submitEventCreate}>
          <div class="field">
            <label for="ev-name">Name</label>
            <input id="ev-name" type="text" bind:value={evCreate.name} required />
          </div>
          <div class="field-row">
            <div class="field">
              <label for="ev-seq">Sequence</label>
              <input id="ev-seq" type="number" min="1" bind:value={evCreate.sequence} required />
            </div>
            <div class="field">
              <label for="ev-weight">Weight</label>
              <input id="ev-weight" type="number" step="0.1" bind:value={evCreate.weight} />
            </div>
          </div>
          <div class="field-row">
            <div class="field">
              <label for="ev-starts">Starts at</label>
              <input id="ev-starts" type="datetime-local" bind:value={evCreate.starts_at} required />
            </div>
            <div class="field">
              <label for="ev-ends">Ends at</label>
              <input id="ev-ends" type="datetime-local" bind:value={evCreate.ends_at} required />
            </div>
          </div>
          {#if evCreateError}<p class="form-error">{evCreateError}</p>{/if}
          <div class="form-actions">
            <button type="submit" class="btn-primary" disabled={evCreating}>
              {evCreating ? 'Adding…' : 'Add event'}
            </button>
            <button type="button" class="btn-secondary" onclick={() => (eventCreating = false)}>Cancel</button>
          </div>
        </form>
      {/if}

      {#if events.length === 0 && !eventCreating}
        <p class="muted">No events yet — add one above.</p>
      {:else}
        <ul class="rows">
          {#each events as ev (ev.id)}
            <li>
              {#if editingEventId === ev.id}
                <form onsubmit={submitEventEdit}>
                  <div class="field">
                    <label for="ev-edit-name">Name</label>
                    <input id="ev-edit-name" type="text" bind:value={evEdit.name} required />
                  </div>
                  <div class="field-row">
                    <div class="field">
                      <label for="ev-edit-seq">Sequence</label>
                      <input id="ev-edit-seq" type="number" min="1" bind:value={evEdit.sequence} required />
                    </div>
                    <div class="field">
                      <label for="ev-edit-weight">Weight</label>
                      <input id="ev-edit-weight" type="number" step="0.1" bind:value={evEdit.weight} />
                    </div>
                  </div>
                  <div class="field-row">
                    <div class="field">
                      <label for="ev-edit-starts">Starts at</label>
                      <input id="ev-edit-starts" type="datetime-local" bind:value={evEdit.starts_at} required />
                    </div>
                    <div class="field">
                      <label for="ev-edit-ends">Ends at</label>
                      <input id="ev-edit-ends" type="datetime-local" bind:value={evEdit.ends_at} required />
                    </div>
                  </div>
                  {#if evEditError}<p class="form-error">{evEditError}</p>{/if}
                  <div class="form-actions">
                    <button type="submit" class="btn-primary" disabled={evSaving}>
                      {evSaving ? 'Saving…' : 'Save'}
                    </button>
                    <button type="button" class="btn-secondary" onclick={() => (editingEventId = null)}>Cancel</button>
                  </div>
                </form>
              {:else}
                <div class="row">
                  <div>
                    <span class="seq">#{ev.sequence}</span>
                    <strong>{ev.name}</strong>
                    {#if ev.scoring_rule_override}
                      <span class="badge">override: {ev.scoring_rule_override}</span>
                    {/if}
                    <div class="row-meta muted">
                      {fmtDateTime(ev.starts_at)} → {fmtDateTime(ev.ends_at)} · weight {ev.weight}
                    </div>
                  </div>
                  <button type="button" class="btn-link" onclick={() => openEventEdit(ev)}>Edit</button>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <!-- ── Categories ─────────────────────────────────────── -->
    <section class="card">
      <div class="card-head">
        <h2>Categories ({categories.length})</h2>
        {#if !categoryCreating}
          <button type="button" class="btn-link" onclick={openCategoryCreate}>+ Add category</button>
        {/if}
      </div>

      {#if categoryCreating}
        <form onsubmit={submitCategoryCreate}>
          <div class="field-row">
            <div class="field">
              <label for="cat-name">Name</label>
              <input id="cat-name" type="text" bind:value={catCreate.name} required />
            </div>
            <div class="field">
              <label for="cat-sort">Sort order</label>
              <input id="cat-sort" type="number" bind:value={catCreate.sort_order} />
            </div>
          </div>
          {#if catCreateError}<p class="form-error">{catCreateError}</p>{/if}
          <div class="form-actions">
            <button type="submit" class="btn-primary" disabled={catCreating}>
              {catCreating ? 'Adding…' : 'Add category'}
            </button>
            <button type="button" class="btn-secondary" onclick={() => (categoryCreating = false)}>Cancel</button>
          </div>
        </form>
      {/if}

      {#if categories.length === 0 && !categoryCreating}
        <p class="muted">No categories yet — climbers can't register until you add at least one.</p>
      {:else}
        <ul class="rows">
          {#each categories as cat (cat.id)}
            <li class="row">
              <div>
                <strong>{cat.name}</strong>
                <span class="muted">order {cat.sort_order}</span>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
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
    font-size: 1.05rem;
    margin: 0;
    color: #334155;
  }
  .meta {
    color: #475569;
    margin: 0.25rem 0;
  }
  .quick-links {
    display: flex;
    gap: 1rem;
    margin: 0.75rem 0 1.5rem;
    font-weight: 600;
  }
  .quick-links a {
    color: var(--rw-accent);
    text-decoration: none;
  }
  .quick-links a:hover {
    text-decoration: underline;
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
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
  }
  .card {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 12px;
    padding: 1.25rem;
    margin-bottom: 1.5rem;
  }
  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
  }
  .btn-link {
    background: none;
    border: 0;
    color: var(--rw-accent);
    font-weight: 600;
    cursor: pointer;
    font-size: 0.9rem;
  }
  .btn-link:hover {
    text-decoration: underline;
  }
  dl.kv {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 1rem;
    margin: 0;
  }
  dl.kv dt {
    color: #64748b;
    font-size: 0.85rem;
  }
  dl.kv dd {
    margin: 0;
  }
  ul.rows {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  ul.rows li {
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    padding: 0.75rem 1rem;
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
  }
  .row-meta {
    font-size: 0.85rem;
    margin-top: 4px;
  }
  .seq {
    color: #94a3b8;
    margin-right: 6px;
  }
  .badge {
    display: inline-block;
    background: #fef3c7;
    color: #92400e;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 600;
    margin-left: 6px;
  }
  .muted {
    color: #94a3b8;
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
  .field {
    margin-bottom: 0.75rem;
  }
  .field label {
    display: block;
    font-size: 0.85rem;
    font-weight: 600;
    color: #334155;
    margin-bottom: 4px;
  }
  .field input,
  .field select {
    width: 100%;
    padding: 0.5rem 0.7rem;
    border: 1px solid #cbd5e1;
    border-radius: 6px;
    font-size: 0.95rem;
    box-sizing: border-box;
  }
  .field-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }
  .form-actions {
    display: flex;
    gap: 8px;
    margin-top: 0.75rem;
  }
  .form-actions .btn-primary {
    padding: 0.55rem 1rem;
    background: var(--rw-accent);
    color: #fff;
    border: 0;
    border-radius: 6px;
    font-weight: 600;
    cursor: pointer;
  }
  .form-actions .btn-primary:disabled {
    background: #fbbf24;
  }
  .form-actions .btn-secondary {
    padding: 0.55rem 1rem;
    background: #fff;
    color: #475569;
    border: 1px solid #cbd5e1;
    border-radius: 6px;
    font-weight: 600;
    cursor: pointer;
  }
  .form-error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    margin: 0.5rem 0 0;
    font-size: 0.85rem;
  }
</style>
