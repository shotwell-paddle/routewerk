<script lang="ts">
  import { onMount } from 'svelte';
  import {
    listMyOrgs,
    getOrg,
    updateOrg,
    listOrgLocations,
    createLocation,
    updateLocation,
    ApiClientError,
    type OrgShape,
    type LocationShape,
    type OrgUpdateShape,
    type LocationCreateShape,
  } from '$lib/api/client';
  import { authState, roleRankAt } from '$lib/stores/auth.svelte';

  // Org admin acts at the org level, not a single location. We let the user
  // pick which org to admin; for single-org users this is a one-option
  // dropdown that auto-selects.
  let orgs = $state<OrgShape[]>([]);
  let selectedOrgId = $state('');
  let org = $state<OrgShape | null>(null);
  let locations = $state<LocationShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  // Membership-derived role at the org. The endpoints all enforce on the
  // server; client gating just hides write affordances the user can't use.
  const isOrgAdmin = $derived.by(() => {
    const me = authState().me;
    if (!me || !selectedOrgId) return false;
    if (me.user.is_app_admin) return true;
    return me.memberships.some(
      (m) => m.org_id === selectedOrgId && m.role === 'org_admin',
    );
  });

  let orgForm = $state({ name: '', slug: '', logo_url: '' });
  let orgSaving = $state(false);
  let orgSaveOk = $state<string | null>(null);
  let orgSaveError = $state<string | null>(null);

  let editingLocId = $state<string | null>(null);
  let creatingLoc = $state(false);
  let locForm = $state<LocationCreateShape>({
    name: '',
    slug: '',
    timezone: 'America/New_York',
    address: '',
    website_url: '',
    phone: '',
    day_pass_info: '',
    waiver_url: '',
    allow_shared_setters: false,
  });
  let locSaving = $state(false);
  let locError = $state<string | null>(null);

  onMount(async () => {
    try {
      orgs = await listMyOrgs();
      if (orgs.length === 0) {
        loading = false;
        return;
      }
      // Default to the first org. The picker swaps without reloading.
      selectedOrgId = orgs[0].id;
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load orgs.';
      loading = false;
    }
  });

  $effect(() => {
    if (!selectedOrgId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getOrg(selectedOrgId),
      listOrgLocations(selectedOrgId).catch(() => [] as LocationShape[]),
    ])
      .then(([o, locs]) => {
        if (cancelled) return;
        org = o;
        locations = locs;
        orgForm = { name: o.name, slug: o.slug, logo_url: o.logo_url ?? '' };
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load org.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function saveOrg(e: Event) {
    e.preventDefault();
    if (!selectedOrgId) return;
    orgSaving = true;
    orgSaveOk = null;
    orgSaveError = null;
    const body: OrgUpdateShape = {
      name: orgForm.name.trim(),
      slug: orgForm.slug.trim(),
      logo_url: orgForm.logo_url.trim() || null,
    };
    try {
      org = await updateOrg(selectedOrgId, body);
      orgSaveOk = 'Saved.';
    } catch (err) {
      orgSaveError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      orgSaving = false;
    }
  }

  function openCreateLoc() {
    editingLocId = null;
    creatingLoc = true;
    locForm = {
      name: '',
      slug: '',
      timezone: 'America/New_York',
      address: '',
      website_url: '',
      phone: '',
      day_pass_info: '',
      waiver_url: '',
      allow_shared_setters: false,
    };
    locError = null;
  }

  function openEditLoc(loc: LocationShape) {
    creatingLoc = false;
    editingLocId = loc.id;
    locForm = {
      name: loc.name,
      slug: loc.slug,
      timezone: loc.timezone,
      address: loc.address ?? '',
      website_url: loc.website_url ?? '',
      phone: loc.phone ?? '',
      // day_pass_info + waiver_url + allow_shared_setters aren't on the
      // basic LocationShape (the SPA only surfaced the columns it needed
      // for the picker). They're optional on PUT; leave empty so we don't
      // silently overwrite values we never read.
      day_pass_info: '',
      waiver_url: '',
      allow_shared_setters: false,
    };
    locError = null;
  }

  function closeLoc() {
    editingLocId = null;
    creatingLoc = false;
    locError = null;
  }

  async function submitLoc(e: Event) {
    e.preventDefault();
    if (!selectedOrgId) return;
    locSaving = true;
    locError = null;
    try {
      // Strip empty optional strings so we don't write empty addresses etc.
      const body: LocationCreateShape = {
        name: (locForm.name ?? '').trim(),
        slug: (locForm.slug ?? '').trim() || undefined,
        timezone: (locForm.timezone ?? '').trim() || undefined,
        address: locForm.address?.trim() || null,
        website_url: locForm.website_url?.trim() || null,
        phone: locForm.phone?.trim() || null,
        day_pass_info: locForm.day_pass_info?.trim() || null,
        waiver_url: locForm.waiver_url?.trim() || null,
        allow_shared_setters: !!locForm.allow_shared_setters,
      };
      if (editingLocId) {
        await updateLocation(editingLocId, body);
      } else {
        await createLocation(selectedOrgId, body);
      }
      // Refresh the list.
      locations = await listOrgLocations(selectedOrgId);
      closeLoc();
    } catch (err) {
      locError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      locSaving = false;
    }
  }
</script>

<svelte:head>
  <title>Organization — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/settings">← Settings</a>
  <h1>Organization</h1>
  <p class="lede">Org-level settings + the gyms that belong to this org.</p>

  {#if orgs.length > 1}
    <label class="org-picker">
      <span>Organization</span>
      <select bind:value={selectedOrgId}>
        {#each orgs as o (o.id)}<option value={o.id}>{o.name}</option>{/each}
      </select>
    </label>
  {/if}

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if !isOrgAdmin}
    <p class="muted">
      Only organization admins can edit organization settings. The endpoints
      below are read-only for you.
    </p>
  {:else if org}
    <section class="card">
      <h2>Organization details</h2>
      <form onsubmit={saveOrg}>
        <label>
          <span>Name</span>
          <input bind:value={orgForm.name} required />
        </label>
        <label>
          <span>Slug</span>
          <input bind:value={orgForm.slug} required pattern="[a-z0-9-]+" />
          <span class="hint muted">URL-safe (lowercase letters, digits, hyphens).</span>
        </label>
        <label>
          <span>Logo URL</span>
          <input type="url" bind:value={orgForm.logo_url} placeholder="https://…" />
        </label>
        {#if orgSaveError}<p class="error">{orgSaveError}</p>{/if}
        {#if orgSaveOk}<p class="ok">{orgSaveOk}</p>{/if}
        <div class="actions">
          <button class="primary" type="submit" disabled={orgSaving}>
            {orgSaving ? 'Saving…' : 'Save org'}
          </button>
        </div>
      </form>
    </section>

    <section class="card">
      <div class="card-head">
        <h2>Gyms ({locations.length})</h2>
        {#if !creatingLoc && !editingLocId}
          <button class="primary" onclick={openCreateLoc}>+ New gym</button>
        {/if}
      </div>

      {#if creatingLoc || editingLocId}
        <form class="loc-form" onsubmit={submitLoc}>
          <h3>{editingLocId ? 'Edit gym' : 'New gym'}</h3>
          <div class="row">
            <label>
              <span>Name *</span>
              <input bind:value={locForm.name} required />
            </label>
            <label>
              <span>Slug</span>
              <input bind:value={locForm.slug} placeholder="auto from name" pattern="[a-z0-9-]*" />
            </label>
          </div>
          <div class="row">
            <label>
              <span>Timezone</span>
              <input bind:value={locForm.timezone} />
            </label>
            <label>
              <span>Phone</span>
              <input bind:value={locForm.phone} />
            </label>
          </div>
          <label>
            <span>Address</span>
            <input bind:value={locForm.address} />
          </label>
          <label>
            <span>Website</span>
            <input type="url" bind:value={locForm.website_url} placeholder="https://…" />
          </label>
          <label>
            <span>Waiver URL</span>
            <input type="url" bind:value={locForm.waiver_url} placeholder="https://…" />
          </label>
          <label>
            <span>Day pass info</span>
            <textarea bind:value={locForm.day_pass_info} rows="2"></textarea>
          </label>
          <label class="check">
            <input type="checkbox" bind:checked={locForm.allow_shared_setters} />
            Allow setters from other gyms in this org to work here
          </label>
          {#if locError}<p class="error">{locError}</p>{/if}
          <div class="actions">
            <button class="primary" type="submit" disabled={locSaving}>
              {locSaving ? 'Saving…' : editingLocId ? 'Save gym' : 'Create gym'}
            </button>
            <button type="button" onclick={closeLoc} disabled={locSaving}>Cancel</button>
          </div>
        </form>
      {/if}

      {#if locations.length === 0}
        <p class="muted">No gyms in this org yet.</p>
      {:else}
        <ul class="loc-list">
          {#each locations as l (l.id)}
            <li>
              <div class="loc-row">
                <div>
                  <div class="loc-name">{l.name}</div>
                  <div class="loc-meta muted">
                    {l.slug} · {l.timezone}
                    {#if l.phone}· {l.phone}{/if}
                  </div>
                </div>
                <button onclick={() => openEditLoc(l)}>Edit</button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {/if}
</div>

<style>
  .page {
    max-width: 48rem;
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
  h3 {
    font-size: 0.95rem;
    font-weight: 600;
    margin: 0 0 0.6rem;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0 0 1.25rem;
  }
  .org-picker {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 1rem;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--rw-text-muted);
    max-width: 18rem;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .card h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.75rem;
  }
  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }
  form label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 0.85rem;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--rw-text-muted);
  }
  label.check {
    flex-direction: row;
    align-items: center;
    gap: 8px;
    color: var(--rw-text);
    font-weight: 500;
  }
  input,
  textarea,
  select {
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    box-sizing: border-box;
    font-family: inherit;
  }
  input:focus,
  textarea:focus,
  select:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.85rem;
  }
  .row label {
    margin-bottom: 0.85rem;
  }
  .actions {
    display: flex;
    gap: 8px;
    margin-top: 0.5rem;
  }
  button {
    cursor: pointer;
    padding: 0.5rem 1rem;
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
  .loc-form {
    border-top: 1px dashed var(--rw-border);
    padding-top: 1rem;
    margin-bottom: 1rem;
  }
  .loc-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .loc-list li {
    border-top: 1px solid var(--rw-border);
    padding-top: 8px;
  }
  .loc-list li:first-child {
    border-top: none;
  }
  .loc-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }
  .loc-name {
    font-weight: 600;
  }
  .loc-meta {
    font-size: 0.85rem;
    margin-top: 2px;
  }
  .hint {
    font-size: 0.75rem;
    margin: 4px 0 0;
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
    margin: 0.5rem 0;
  }
  .ok {
    background: rgba(22, 163, 74, 0.1);
    border: 1px solid rgba(22, 163, 74, 0.3);
    color: #15803d;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.5rem 0;
  }
</style>
