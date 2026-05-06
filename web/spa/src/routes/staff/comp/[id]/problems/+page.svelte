<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    listEvents,
    listProblems,
    createProblem,
    updateProblem,
    type Competition,
    type CompetitionEvent,
    type CompetitionProblem,
    type ProblemCreate,
    type ProblemUpdate,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let events = $state<CompetitionEvent[]>([]);
  // Problems keyed by event id so we can refresh per-event after a write.
  let problemsByEvent = $state<Record<string, CompetitionProblem[]>>({});
  let loading = $state(true);
  let error = $state<string | null>(null);

  // One create-form open at a time keeps the page focused.
  let creatingForEventId = $state<string | null>(null);
  let editingProblemId = $state<string | null>(null);

  const id = $derived(page.params.id ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/staff/comp/' + id + '/problems'));
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;
    try {
      const [c, evs] = await Promise.all([getCompetition(id), listEvents(id)]);
      comp = c;
      events = evs;
      const lists = await Promise.all(evs.map((e) => listProblems(e.id)));
      const next: Record<string, CompetitionProblem[]> = {};
      evs.forEach((e, i) => (next[e.id] = lists[i]));
      problemsByEvent = next;
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load competition.';
    } finally {
      loading = false;
    }
  });

  async function refreshEventProblems(eventId: string) {
    problemsByEvent = { ...problemsByEvent, [eventId]: await listProblems(eventId) };
  }

  // ── Create form ───────────────────────────────────────────

  let createForm = $state({
    label: '',
    grade: '',
    color: '',
    points: '',
    zone_points: '',
    sort_order: '',
  });
  let createError = $state<string | null>(null);
  let createSaving = $state(false);

  function openCreate(eventId: string) {
    editingProblemId = null;
    creatingForEventId = eventId;
    const list = problemsByEvent[eventId] ?? [];
    const nextSort = list.length === 0 ? 1 : Math.max(...list.map((p) => p.sort_order)) + 1;
    createForm = {
      label: '',
      grade: '',
      color: '',
      points: '',
      zone_points: '',
      sort_order: String(nextSort),
    };
    createError = null;
  }

  function closeCreate() {
    creatingForEventId = null;
    createError = null;
  }

  async function submitCreate(eventId: string) {
    createSaving = true;
    createError = null;
    try {
      const body: ProblemCreate = {
        label: createForm.label.trim(),
        sort_order: parseInt(createForm.sort_order, 10) || 0,
      };
      if (createForm.grade.trim()) body.grade = createForm.grade.trim();
      if (createForm.color.trim()) body.color = createForm.color.trim();
      if (createForm.points.trim()) body.points = parseFloat(createForm.points);
      if (createForm.zone_points.trim()) body.zone_points = parseFloat(createForm.zone_points);
      await createProblem(eventId, body);
      await refreshEventProblems(eventId);
      closeCreate();
    } catch (err) {
      createError = err instanceof ApiClientError ? err.message : 'Could not create problem.';
    } finally {
      createSaving = false;
    }
  }

  // ── Edit form ─────────────────────────────────────────────

  let editForm = $state({
    label: '',
    grade: '',
    color: '',
    points: '',
    zone_points: '',
    sort_order: '',
  });
  let editError = $state<string | null>(null);
  let editSaving = $state(false);

  function openEdit(p: CompetitionProblem) {
    creatingForEventId = null;
    editingProblemId = p.id;
    editForm = {
      label: p.label,
      grade: p.grade ?? '',
      color: p.color ?? '',
      points: p.points != null ? String(p.points) : '',
      zone_points: p.zone_points != null ? String(p.zone_points) : '',
      sort_order: String(p.sort_order),
    };
    editError = null;
  }

  function closeEdit() {
    editingProblemId = null;
    editError = null;
  }

  async function submitEdit(p: CompetitionProblem) {
    editSaving = true;
    editError = null;
    try {
      const body: ProblemUpdate = {
        label: editForm.label.trim(),
        sort_order: parseInt(editForm.sort_order, 10) || 0,
        grade: editForm.grade.trim() ? editForm.grade.trim() : null,
        color: editForm.color.trim() ? editForm.color.trim() : null,
        points: editForm.points.trim() ? parseFloat(editForm.points) : null,
        zone_points: editForm.zone_points.trim() ? parseFloat(editForm.zone_points) : null,
      };
      await updateProblem(p.id, body);
      await refreshEventProblems(p.event_id);
      closeEdit();
    } catch (err) {
      editError = err instanceof ApiClientError ? err.message : 'Could not update problem.';
    } finally {
      editSaving = false;
    }
  }

  // ── Reorder ───────────────────────────────────────────────

  // Swap a problem's sort_order with its neighbor and persist both via PATCH.
  // Optimistic-free: refresh from server after both writes resolve so we
  // don't have to reason about partial-failure UI.
  let reorderingId = $state<string | null>(null);

  async function move(p: CompetitionProblem, direction: -1 | 1) {
    const list = (problemsByEvent[p.event_id] ?? [])
      .slice()
      .sort((a, b) => a.sort_order - b.sort_order);
    const idx = list.findIndex((x) => x.id === p.id);
    const targetIdx = idx + direction;
    if (idx < 0 || targetIdx < 0 || targetIdx >= list.length) return;
    const other = list[targetIdx];
    reorderingId = p.id;
    try {
      await Promise.all([
        updateProblem(p.id, { sort_order: other.sort_order }),
        updateProblem(other.id, { sort_order: p.sort_order }),
      ]);
      await refreshEventProblems(p.event_id);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Reorder failed.';
    } finally {
      reorderingId = null;
    }
  }

  function sorted(list: CompetitionProblem[] | undefined): CompetitionProblem[] {
    return (list ?? []).slice().sort((a, b) => a.sort_order - b.sort_order);
  }
</script>

<svelte:head>
  <title>Problems — {comp?.name ?? 'Competition'} — Routewerk</title>
</svelte:head>

<main>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <h1>Couldn't load competition</h1>
    <p class="error">{error}</p>
    <p><a href="/staff/comp">← Back to competitions</a></p>
  {:else if comp}
    <header>
      <a class="back" href="/staff/comp/{id}">← {comp.name}</a>
      <h1>Problems</h1>
      <p class="meta">
        Per-event problem editor. Scoring rule: <code>{comp.scoring_rule}</code>.
      </p>
    </header>

    {#if events.length === 0}
      <p class="muted">No events yet. Add an event from the comp dashboard first.</p>
    {:else}
      <ul class="event-list">
        {#each events as ev (ev.id)}
          {@const list = sorted(problemsByEvent[ev.id])}
          <li class="event">
            <div class="event-head">
              <span class="seq">#{ev.sequence}</span>
              <span class="name">{ev.name}</span>
              <span class="count">{list.length} problem{list.length === 1 ? '' : 's'}</span>
              {#if creatingForEventId !== ev.id}
                <button class="add" onclick={() => openCreate(ev.id)}>+ Add problem</button>
              {/if}
            </div>

            {#if creatingForEventId === ev.id}
              <div class="form">
                <h4>New problem</h4>
                <div class="field-row">
                  <div class="field">
                    <label for="c-label-{ev.id}">Label *</label>
                    <input id="c-label-{ev.id}" bind:value={createForm.label} placeholder="M1, B7…" />
                  </div>
                  <div class="field">
                    <label for="c-sort-{ev.id}">Sort order</label>
                    <input id="c-sort-{ev.id}" type="number" bind:value={createForm.sort_order} />
                  </div>
                </div>
                <div class="field-row">
                  <div class="field">
                    <label for="c-grade-{ev.id}">Grade</label>
                    <input id="c-grade-{ev.id}" bind:value={createForm.grade} placeholder="V4, 5.11a…" />
                  </div>
                  <div class="field">
                    <label for="c-color-{ev.id}">Color</label>
                    <input id="c-color-{ev.id}" bind:value={createForm.color} placeholder="red, blue…" />
                  </div>
                </div>
                {#if comp.scoring_rule === 'fixed' || comp.scoring_rule === 'decay'}
                  <div class="field-row">
                    <div class="field">
                      <label for="c-points-{ev.id}">Points</label>
                      <input id="c-points-{ev.id}" type="number" step="0.01" bind:value={createForm.points} />
                    </div>
                    <div class="field">
                      <label for="c-zpoints-{ev.id}">Zone points</label>
                      <input id="c-zpoints-{ev.id}" type="number" step="0.01" bind:value={createForm.zone_points} />
                    </div>
                  </div>
                {/if}
                {#if createError}<p class="error">{createError}</p>{/if}
                <div class="form-actions">
                  <button class="primary" disabled={createSaving || !createForm.label.trim()} onclick={() => submitCreate(ev.id)}>
                    {createSaving ? 'Saving…' : 'Create'}
                  </button>
                  <button onclick={closeCreate} disabled={createSaving}>Cancel</button>
                </div>
              </div>
            {/if}

            {#if list.length === 0}
              <p class="empty muted">No problems yet.</p>
            {:else}
              <ol class="problem-list">
                {#each list as p, i (p.id)}
                  <li class="problem">
                    {#if editingProblemId === p.id}
                      <div class="form">
                        <h4>Edit {p.label}</h4>
                        <div class="field-row">
                          <div class="field">
                            <label for="e-label-{p.id}">Label *</label>
                            <input id="e-label-{p.id}" bind:value={editForm.label} />
                          </div>
                          <div class="field">
                            <label for="e-sort-{p.id}">Sort order</label>
                            <input id="e-sort-{p.id}" type="number" bind:value={editForm.sort_order} />
                          </div>
                        </div>
                        <div class="field-row">
                          <div class="field">
                            <label for="e-grade-{p.id}">Grade</label>
                            <input id="e-grade-{p.id}" bind:value={editForm.grade} />
                          </div>
                          <div class="field">
                            <label for="e-color-{p.id}">Color</label>
                            <input id="e-color-{p.id}" bind:value={editForm.color} />
                          </div>
                        </div>
                        <div class="field-row">
                          <div class="field">
                            <label for="e-points-{p.id}">Points</label>
                            <input id="e-points-{p.id}" type="number" step="0.01" bind:value={editForm.points} />
                          </div>
                          <div class="field">
                            <label for="e-zpoints-{p.id}">Zone points</label>
                            <input id="e-zpoints-{p.id}" type="number" step="0.01" bind:value={editForm.zone_points} />
                          </div>
                        </div>
                        {#if editError}<p class="error">{editError}</p>{/if}
                        <div class="form-actions">
                          <button class="primary" disabled={editSaving || !editForm.label.trim()} onclick={() => submitEdit(p)}>
                            {editSaving ? 'Saving…' : 'Save'}
                          </button>
                          <button onclick={closeEdit} disabled={editSaving}>Cancel</button>
                        </div>
                      </div>
                    {:else}
                      <div class="problem-row">
                        <span class="p-sort">#{p.sort_order}</span>
                        <span class="p-label">{p.label}</span>
                        <span class="p-meta">
                          {#if p.grade}{p.grade}{/if}
                          {#if p.color}<span class="dot" style="background:{p.color}" title={p.color}></span>{/if}
                          {#if p.points != null}· {p.points}pt{/if}
                          {#if p.zone_points != null}· z{p.zone_points}{/if}
                        </span>
                        <span class="p-actions">
                          <button
                            class="icon"
                            disabled={i === 0 || reorderingId !== null}
                            onclick={() => move(p, -1)}
                            aria-label="Move up"
                            title="Move up">↑</button>
                          <button
                            class="icon"
                            disabled={i === list.length - 1 || reorderingId !== null}
                            onclick={() => move(p, 1)}
                            aria-label="Move down"
                            title="Move down">↓</button>
                          <button class="link" onclick={() => openEdit(p)}>Edit</button>
                        </span>
                      </div>
                    {/if}
                  </li>
                {/each}
              </ol>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</main>

<style>
  main {
    max-width: 56rem;
    margin: 0 auto;
    padding: 1.5rem 1rem 4rem;
  }
  .back {
    display: inline-block;
    color: var(--rw-accent);
    text-decoration: none;
    margin-bottom: 0.75rem;
    font-weight: 600;
  }
  h1 {
    font-size: 1.6rem;
    margin: 0 0 0.5rem;
  }
  .meta {
    color: #475569;
    margin: 0 0 1.5rem;
  }
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
  }
  .event-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .event {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    padding: 1rem 1rem 0.5rem;
  }
  .event-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 0.75rem;
    flex-wrap: wrap;
  }
  .seq {
    color: #94a3b8;
    font-weight: 600;
    font-size: 0.85rem;
  }
  .name {
    font-weight: 600;
    font-size: 1.05rem;
    flex: 1;
  }
  .count {
    color: #64748b;
    font-size: 0.85rem;
  }
  .empty {
    margin: 0 0 0.5rem;
    font-size: 0.9rem;
  }
  .problem-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .problem {
    border-top: 1px solid #f1f5f9;
    padding-top: 6px;
  }
  .problem-row {
    display: grid;
    grid-template-columns: 3rem 4rem 1fr auto;
    gap: 8px;
    align-items: center;
    padding: 0.4rem 0;
  }
  .p-sort {
    color: #94a3b8;
    font-weight: 600;
    font-size: 0.85rem;
  }
  .p-label {
    font-weight: 700;
  }
  .p-meta {
    color: #64748b;
    font-size: 0.9rem;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .dot {
    display: inline-block;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    border: 1px solid #cbd5e1;
  }
  .p-actions {
    display: inline-flex;
    gap: 4px;
  }
  .form {
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    padding: 0.85rem;
    margin: 0.5rem 0;
  }
  .form h4 {
    margin: 0 0 0.5rem;
    font-size: 0.9rem;
    color: #334155;
  }
  .field {
    margin-bottom: 0.6rem;
  }
  .field label {
    display: block;
    font-size: 0.8rem;
    font-weight: 600;
    color: #475569;
    margin-bottom: 3px;
  }
  .field input {
    width: 100%;
    padding: 0.45rem 0.65rem;
    border: 1px solid #cbd5e1;
    border-radius: 6px;
    font-size: 0.95rem;
    box-sizing: border-box;
  }
  .field-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
  }
  .form-actions {
    display: flex;
    gap: 8px;
    margin-top: 0.4rem;
  }
  button {
    cursor: pointer;
    padding: 0.45rem 0.85rem;
    border-radius: 6px;
    border: 1px solid #cbd5e1;
    background: #fff;
    font-size: 0.9rem;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  button.primary {
    background: var(--rw-accent);
    color: #fff;
    border-color: var(--rw-accent);
    font-weight: 600;
  }
  button.add {
    border-color: var(--rw-accent);
    color: var(--rw-accent);
    font-weight: 600;
    font-size: 0.85rem;
    padding: 0.3rem 0.65rem;
  }
  button.icon {
    padding: 0.2rem 0.5rem;
    font-size: 0.85rem;
    line-height: 1;
  }
  button.link {
    border: none;
    background: transparent;
    color: var(--rw-accent);
    padding: 0.2rem 0.4rem;
    font-weight: 600;
    font-size: 0.85rem;
  }
  .muted {
    color: #94a3b8;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.4rem 0;
  }
</style>
