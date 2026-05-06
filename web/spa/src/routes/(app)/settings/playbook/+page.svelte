<script lang="ts">
  import { goto } from '$app/navigation';
  import {
    listPlaybookSteps,
    createPlaybookStep,
    updatePlaybookStep,
    deletePlaybookStep,
    movePlaybookStep,
    ApiClientError,
    type PlaybookStepShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { authState, roleRankAt } from '$lib/stores/auth.svelte';

  // Setter playbook editor — the default checklist template applied
  // to new sessions. Mirrors the HTMX /settings/playbook page; the
  // session lifecycle UI snapshots these steps when a session is
  // created so deleting / reordering doesn't cascade into past
  // sessions.

  const locId = $derived(effectiveLocationId());
  const canEdit = $derived(roleRankAt(locId) >= 3);

  let steps = $state<PlaybookStepShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let mutatingId = $state<string | null>(null);

  // Append form state.
  let newTitle = $state('');
  let creating = $state(false);

  // Inline edit state.
  let editingId = $state<string | null>(null);
  let editTitle = $state('');

  // Page-level role gate. Redirect anything below head_setter to the
  // settings index so they don't sit on a permission-denied page.
  $effect(() => {
    if (!authState().loaded || !authState().me) return;
    if (!canEdit) {
      goto('/settings');
    }
  });

  async function refresh() {
    if (!locId) return;
    steps = await listPlaybookSteps(locId);
  }

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    refresh()
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load playbook.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function append(e: Event) {
    e.preventDefault();
    if (!locId || creating) return;
    const v = newTitle.trim();
    if (!v) return;
    creating = true;
    error = null;
    try {
      await createPlaybookStep(locId, v);
      newTitle = '';
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not add step.';
    } finally {
      creating = false;
    }
  }

  function openEdit(step: PlaybookStepShape) {
    editingId = step.id;
    editTitle = step.title;
  }

  async function saveEdit(stepId: string) {
    if (!locId || mutatingId) return;
    const v = editTitle.trim();
    if (!v) return;
    mutatingId = stepId;
    error = null;
    try {
      await updatePlaybookStep(locId, stepId, v);
      // Update locally without re-fetching so the row stays in place.
      steps = steps.map((s) => (s.id === stepId ? { ...s, title: v } : s));
      editingId = null;
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not save step.';
    } finally {
      mutatingId = null;
    }
  }

  async function remove(step: PlaybookStepShape) {
    if (!locId || mutatingId) return;
    if (!confirm(`Delete step "${step.title}"? Existing session checklists keep their copy of this step.`)) return;
    mutatingId = step.id;
    error = null;
    try {
      await deletePlaybookStep(locId, step.id);
      steps = steps.filter((s) => s.id !== step.id);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not delete step.';
    } finally {
      mutatingId = null;
    }
  }

  async function move(step: PlaybookStepShape, direction: 'up' | 'down') {
    if (!locId || mutatingId) return;
    mutatingId = step.id;
    error = null;
    try {
      await movePlaybookStep(locId, step.id, direction);
      await refresh();
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not reorder step.';
    } finally {
      mutatingId = null;
    }
  }
</script>

<svelte:head>
  <title>Setter playbook — Routewerk</title>
</svelte:head>

<div class="page">
  <p><a class="back" href="/settings">← Settings</a></p>
  <h1>Setter playbook</h1>
  <p class="lede">
    Default checklist applied to new sessions when "default playbook
    enabled" is on in gym settings. Existing sessions snapshot their
    steps at create time, so editing here only affects future sessions.
  </p>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if !canEdit}
    <p class="muted">Head setter or above required.</p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else}
    {#if error}<p class="error">{error}</p>{/if}

    <section class="card">
      <h2>Steps ({steps.length})</h2>
      {#if steps.length === 0}
        <p class="muted">No steps yet. Add one below to seed the template.</p>
      {:else}
        <ul class="step-list">
          {#each steps as step, i (step.id)}
            <li>
              <span class="muted small order">#{i + 1}</span>
              {#if editingId === step.id}
                <form
                  class="edit-form"
                  onsubmit={(e) => { e.preventDefault(); saveEdit(step.id); }}>
                  <!-- svelte-ignore a11y_autofocus -->
                  <input
                    bind:value={editTitle}
                    maxlength="200"
                    autofocus
                    placeholder="Step title" />
                  <button class="primary" type="submit" disabled={mutatingId === step.id}>
                    {mutatingId === step.id ? '…' : 'Save'}
                  </button>
                  <button type="button" disabled={mutatingId === step.id} onclick={() => (editingId = null)}>
                    Cancel
                  </button>
                </form>
              {:else}
                <span class="title">{step.title}</span>
                <span class="actions">
                  <button
                    type="button"
                    class="icon"
                    aria-label="Move up"
                    disabled={i === 0 || mutatingId === step.id}
                    onclick={() => move(step, 'up')}>↑</button>
                  <button
                    type="button"
                    class="icon"
                    aria-label="Move down"
                    disabled={i === steps.length - 1 || mutatingId === step.id}
                    onclick={() => move(step, 'down')}>↓</button>
                  <button
                    type="button"
                    class="ghost"
                    disabled={mutatingId === step.id}
                    onclick={() => openEdit(step)}>
                    Edit
                  </button>
                  <button
                    type="button"
                    class="ghost danger"
                    disabled={mutatingId === step.id}
                    onclick={() => remove(step)}>
                    {mutatingId === step.id ? '…' : 'Delete'}
                  </button>
                </span>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      <form class="append" onsubmit={append}>
        <input
          bind:value={newTitle}
          maxlength="200"
          placeholder="New step (e.g. Sweep mats, Rinse holds, Stage routes)" />
        <button class="primary" type="submit" disabled={creating || !newTitle.trim()}>
          {creating ? '…' : 'Add step'}
        </button>
      </form>
    </section>
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
    margin: 0.25rem 0 0.25rem;
    letter-spacing: -0.01em;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0 0 1.5rem;
    max-width: 36rem;
  }
  .small {
    font-size: 0.8rem;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.1rem 1.25rem;
  }
  .card h2 {
    margin: 0 0 0.75rem;
    font-size: 1rem;
    font-weight: 600;
  }
  .step-list {
    list-style: none;
    padding: 0;
    margin: 0 0 1rem;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .step-list li {
    display: grid;
    grid-template-columns: 2.5rem 1fr auto;
    gap: 10px;
    align-items: center;
    padding: 0.55rem 0;
    border-top: 1px solid var(--rw-border);
  }
  .step-list li:first-child {
    border-top: none;
  }
  .order {
    font-variant-numeric: tabular-nums;
  }
  .title {
    font-weight: 500;
  }
  .actions {
    display: inline-flex;
    gap: 4px;
  }
  .actions button.icon {
    width: 30px;
    padding: 0.2rem 0;
    text-align: center;
    font-size: 1rem;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    border-radius: 6px;
    cursor: pointer;
  }
  .actions button.ghost {
    background: transparent;
    border: 1px solid transparent;
    color: var(--rw-text-muted);
    padding: 0.2rem 0.6rem;
    border-radius: 6px;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
  }
  .actions button.ghost:hover:not(:disabled) {
    border-color: var(--rw-border-strong);
    color: var(--rw-text);
  }
  .actions button.ghost.danger:hover:not(:disabled) {
    color: var(--rw-danger);
    border-color: #fecaca;
  }
  .actions button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .edit-form {
    grid-column: 2 / -1;
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 6px;
  }
  .edit-form input {
    padding: 0.45rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
  }
  .edit-form button {
    padding: 0.4rem 0.85rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-weight: 600;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .edit-form button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
  }
  .append {
    display: flex;
    gap: 8px;
    margin-top: 0.75rem;
  }
  .append input {
    flex: 1;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
  }
  .append button.primary {
    padding: 0.5rem 1rem;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border: 1px solid var(--rw-accent);
    border-radius: 6px;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
  }
  .append button.primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
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
