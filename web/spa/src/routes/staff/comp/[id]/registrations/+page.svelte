<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getCompetition,
    listCategories,
    listRegistrations,
    createRegistration,
    withdrawRegistration,
    type Competition,
    type CompetitionCategory,
    type CompetitionRegistration,
    type RegistrationCreate,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let comp = $state<Competition | null>(null);
  let categories = $state<CompetitionCategory[]>([]);
  let registrations = $state<CompetitionRegistration[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let creating = $state(false);

  const id = $derived(page.params.id ?? '');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/staff/comp/' + id + '/registrations'));
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
    const [c, cats, regs] = await Promise.all([
      getCompetition(id),
      listCategories(id),
      listRegistrations(id),
    ]);
    comp = c;
    categories = cats;
    registrations = regs;
  }

  // Group active (non-withdrawn) registrations by category, in category sort order.
  // Withdrawn live in their own bucket below.
  const grouped = $derived.by(() => {
    const cats = categories.slice().sort((a, b) => a.sort_order - b.sort_order);
    const byCat = new Map<string, CompetitionRegistration[]>();
    cats.forEach((c) => byCat.set(c.id, []));
    const withdrawn: CompetitionRegistration[] = [];
    for (const r of registrations) {
      if (r.withdrawn_at) {
        withdrawn.push(r);
        continue;
      }
      const list = byCat.get(r.category_id) ?? [];
      list.push(r);
      byCat.set(r.category_id, list);
    }
    return {
      cats: cats.map((c) => ({
        cat: c,
        regs: (byCat.get(c.id) ?? []).slice().sort(sortBibThenName),
      })),
      withdrawn: withdrawn.slice().sort(sortBibThenName),
    };
  });

  function sortBibThenName(a: CompetitionRegistration, b: CompetitionRegistration): number {
    if (a.bib_number != null && b.bib_number != null) return a.bib_number - b.bib_number;
    if (a.bib_number != null) return -1;
    if (b.bib_number != null) return 1;
    return a.display_name.localeCompare(b.display_name);
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString();
  }

  // ── Create form ──────────────────────────────────────────

  let createForm = $state({
    category_id: '',
    user_id: '',
    display_name: '',
    bib_number: '',
  });
  let createError = $state<string | null>(null);
  let createSaving = $state(false);

  function openCreate() {
    creating = true;
    createForm = {
      category_id: categories[0]?.id ?? '',
      user_id: '',
      display_name: '',
      bib_number: '',
    };
    createError = null;
  }

  function closeCreate() {
    creating = false;
    createError = null;
  }

  async function submitCreate() {
    if (!createForm.category_id) {
      createError = 'Pick a category.';
      return;
    }
    createSaving = true;
    createError = null;
    try {
      const body: RegistrationCreate = { category_id: createForm.category_id };
      if (createForm.user_id.trim()) body.user_id = createForm.user_id.trim();
      if (createForm.display_name.trim()) body.display_name = createForm.display_name.trim();
      if (createForm.bib_number.trim()) body.bib_number = parseInt(createForm.bib_number, 10);
      const newReg = await createRegistration(id, body);
      registrations = [...registrations, newReg];
      closeCreate();
    } catch (err) {
      createError = err instanceof ApiClientError ? err.message : 'Could not register.';
    } finally {
      createSaving = false;
    }
  }

  // ── Withdraw ─────────────────────────────────────────────

  let withdrawingId = $state<string | null>(null);

  async function withdraw(r: CompetitionRegistration) {
    if (!confirm(`Withdraw ${r.display_name}? Their bib will be freed for reuse.`)) return;
    withdrawingId = r.id;
    try {
      await withdrawRegistration(r.id);
      // Refresh from server — the withdrawn_at field is server-stamped.
      registrations = await listRegistrations(id);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not withdraw.';
    } finally {
      withdrawingId = null;
    }
  }
</script>

<svelte:head>
  <title>Registrations — {comp?.name ?? 'Competition'} — Routewerk</title>
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
      <h1>Registrations</h1>
      <p class="meta">
        {registrations.filter((r) => !r.withdrawn_at).length} active ·
        {registrations.filter((r) => r.withdrawn_at).length} withdrawn
      </p>
      <p class="note muted">
        Per-attempt verification (the league-night queue) lives on the climber
        scorecard / leaderboard, not here. This page is for managing who's in.
      </p>
    </header>

    <section class="card">
      <div class="card-head">
        <h2>Add registration</h2>
        {#if !creating}
          <button class="primary" disabled={categories.length === 0} onclick={openCreate}>
            + Register climber
          </button>
        {/if}
      </div>

      {#if categories.length === 0}
        <p class="muted">Add a category from the comp dashboard before registering climbers.</p>
      {:else if creating}
        <div class="form">
          <div class="field">
            <label for="r-cat">Category *</label>
            <select id="r-cat" bind:value={createForm.category_id}>
              {#each categories as c (c.id)}
                <option value={c.id}>{c.name}</option>
              {/each}
            </select>
          </div>
          <div class="field-row">
            <div class="field">
              <label for="r-user">User UUID</label>
              <input
                id="r-user"
                bind:value={createForm.user_id}
                placeholder="leave blank to register yourself" />
            </div>
            <div class="field">
              <label for="r-bib">Bib number</label>
              <input id="r-bib" type="number" bind:value={createForm.bib_number} />
            </div>
          </div>
          <div class="field">
            <label for="r-name">Display name override</label>
            <input
              id="r-name"
              bind:value={createForm.display_name}
              placeholder="defaults to user's display name" />
          </div>
          {#if createError}<p class="error">{createError}</p>{/if}
          <div class="form-actions">
            <button class="primary" disabled={createSaving} onclick={submitCreate}>
              {createSaving ? 'Saving…' : 'Register'}
            </button>
            <button onclick={closeCreate} disabled={createSaving}>Cancel</button>
          </div>
          <p class="hint muted">
            User-lookup-by-email isn't wired up yet — paste the user UUID for
            now, or register yourself by leaving it blank.
          </p>
        </div>
      {/if}
    </section>

    {#each grouped.cats as { cat, regs } (cat.id)}
      <section class="card">
        <div class="card-head">
          <h2>{cat.name}</h2>
          <span class="count muted">{regs.length} registered</span>
        </div>
        {#if regs.length === 0}
          <p class="empty muted">No active registrations.</p>
        {:else}
          <ul class="reg-list">
            {#each regs as r (r.id)}
              <li class="reg">
                <span class="bib">
                  {#if r.bib_number != null}#{r.bib_number}{:else}—{/if}
                </span>
                <span class="name">{r.display_name}</span>
                <span class="meta-cell">
                  registered {fmtDate(r.created_at)}
                  {#if r.waiver_signed_at}· waiver ✓{/if}
                  {#if r.paid_at}· paid ✓{/if}
                </span>
                <span class="actions">
                  <a class="link" href={`/comp/${comp.id}`} title="Open climber view">Scorecard ↗</a>
                  <button
                    class="danger"
                    disabled={withdrawingId !== null}
                    onclick={() => withdraw(r)}>
                    {withdrawingId === r.id ? 'Withdrawing…' : 'Withdraw'}
                  </button>
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/each}

    {#if grouped.withdrawn.length > 0}
      <section class="card withdrawn-card">
        <div class="card-head">
          <h2>Withdrawn</h2>
          <span class="count muted">{grouped.withdrawn.length}</span>
        </div>
        <ul class="reg-list">
          {#each grouped.withdrawn as r (r.id)}
            <li class="reg withdrawn">
              <span class="bib">
                {#if r.bib_number != null}#{r.bib_number}{:else}—{/if}
              </span>
              <span class="name">{r.display_name}</span>
              <span class="meta-cell">
                withdrew {r.withdrawn_at ? fmtDate(r.withdrawn_at) : ''}
              </span>
            </li>
          {/each}
        </ul>
      </section>
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
  h2 {
    font-size: 1.05rem;
    margin: 0;
  }
  .meta {
    color: #475569;
    margin: 0 0 0.5rem;
  }
  .note {
    font-size: 0.85rem;
    margin: 0 0 1.5rem;
  }
  .card {
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    padding: 1rem;
    margin-bottom: 1rem;
  }
  .withdrawn-card {
    background: #f8fafc;
  }
  .card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 0.75rem;
  }
  .count {
    font-size: 0.85rem;
  }
  .empty {
    margin: 0;
    font-size: 0.9rem;
  }
  .reg-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .reg {
    display: grid;
    grid-template-columns: 3.5rem 1fr 1.5fr auto;
    align-items: center;
    gap: 8px;
    padding: 0.5rem 0.25rem;
    border-top: 1px solid #f1f5f9;
  }
  .reg:first-child {
    border-top: none;
  }
  .reg.withdrawn {
    color: #94a3b8;
    text-decoration: line-through;
  }
  .bib {
    font-weight: 700;
    color: #475569;
  }
  .name {
    font-weight: 600;
  }
  .meta-cell {
    color: #64748b;
    font-size: 0.85rem;
  }
  .actions {
    display: inline-flex;
    gap: 6px;
    align-items: center;
  }
  .form {
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    padding: 0.85rem;
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
  .field input,
  .field select {
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
  .hint {
    font-size: 0.8rem;
    margin: 0.5rem 0 0;
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
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
    background: #fff;
    font-size: 0.85rem;
    padding: 0.3rem 0.65rem;
  }
  button.danger:hover:not(:disabled) {
    background: #fef2f2;
  }
  .link {
    color: var(--rw-accent);
    text-decoration: none;
    font-size: 0.85rem;
    font-weight: 600;
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
