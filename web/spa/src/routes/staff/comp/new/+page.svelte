<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    createCompetition,
    getLocation,
    type CompetitionCreate,
    type CompetitionFormat,
    type LocationShape,
    ApiClientError,
  } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  const STAFF_ROLES = new Set(['head_setter', 'gym_manager', 'org_admin']);
  const SCORERS = ['top_zone', 'fixed', 'decay'] as const;

  interface LocationOption {
    id: string;
    name: string;
  }

  let loading = $state(true);
  let submitting = $state(false);
  let error = $state<string | null>(null);
  let locations = $state<LocationOption[]>([]);

  // Form state
  let locationId = $state('');
  let name = $state('');
  let slug = $state('');
  let slugDirty = $state(false);
  let format = $state<CompetitionFormat>('single' as CompetitionFormat);
  let scoringRule = $state('top_zone');
  let startsAt = $state('');
  let endsAt = $state('');
  let registrationOpensAt = $state('');
  let registrationClosesAt = $state('');

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent('/staff/comp/new'));
    }
  });

  onMount(async () => {
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;

    try {
      const me = authState().me;
      if (!me) return;
      const staffMembers = me.memberships.filter(
        (m) => STAFF_ROLES.has(m.role) && m.location_id,
      );
      const locs = await Promise.all(
        staffMembers.map((m) => getLocation(m.location_id as string)),
      );
      locations = locs
        .filter((l): l is LocationShape => l !== null)
        .map((l) => ({ id: l.id, name: l.name }));
      if (locations.length > 0) {
        locationId = locations[0].id;
      }
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not load locations.';
    } finally {
      loading = false;
    }
  });

  // Auto-derive slug from name unless the user has typed in the field.
  $effect(() => {
    if (!slugDirty) {
      slug = name
        .toLowerCase()
        .replace(/[^a-z0-9-]+/g, '-')
        .replace(/^-+|-+$/g, '')
        .replace(/-{2,}/g, '-')
        .slice(0, 64);
    }
  });

  function onSlugInput(e: Event) {
    const v = (e.currentTarget as HTMLInputElement).value;
    slug = v;
    slugDirty = v.length > 0;
  }

  /** datetime-local "YYYY-MM-DDTHH:mm" → ISO 8601 with TZ. */
  function localToISO(v: string): string | null {
    if (!v) return null;
    const d = new Date(v);
    if (isNaN(d.getTime())) return null;
    return d.toISOString();
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (submitting) return;
    error = null;

    if (!locationId) {
      error = 'Pick a location.';
      return;
    }
    if (!name || !slug) {
      error = 'Name and slug are required.';
      return;
    }
    const startsISO = localToISO(startsAt);
    const endsISO = localToISO(endsAt);
    if (!startsISO || !endsISO) {
      error = 'Start and end dates are required.';
      return;
    }
    if (new Date(endsISO) <= new Date(startsISO)) {
      error = 'End date must be after start date.';
      return;
    }

    const body: CompetitionCreate = {
      name,
      slug,
      format,
      scoring_rule: scoringRule,
      starts_at: startsISO,
      ends_at: endsISO,
    };
    const opens = localToISO(registrationOpensAt);
    const closes = localToISO(registrationClosesAt);
    if (opens) body.registration_opens_at = opens;
    if (closes) body.registration_closes_at = closes;

    submitting = true;
    try {
      const created = await createCompetition(locationId, body);
      goto(`/staff/comp/${created.id}`);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not create competition.';
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>New competition — Routewerk staff</title>
</svelte:head>

<main>
  <p><a href="/staff/comp">← Back to competitions</a></p>
  <h1>New competition</h1>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if locations.length === 0}
    <p class="muted">
      You need <code>head_setter</code> (or higher) at a location to create a competition.
    </p>
  {:else}
    <form onsubmit={submit}>
      <div class="row">
        <label for="location">Location</label>
        <select id="location" bind:value={locationId} required>
          {#each locations as loc (loc.id)}
            <option value={loc.id}>{loc.name}</option>
          {/each}
        </select>
      </div>

      <div class="row">
        <label for="name">Name</label>
        <input id="name" type="text" bind:value={name} required maxlength="120" />
      </div>

      <div class="row">
        <label for="slug">Slug</label>
        <input
          id="slug"
          type="text"
          value={slug}
          oninput={onSlugInput}
          required
          maxlength="64"
          pattern="[a-z0-9][a-z0-9-]*"
        />
        <p class="hint">URL-safe (lowercase letters, digits, hyphens). Auto-derived from name; edit to override.</p>
      </div>

      <div class="row two">
        <div>
          <label for="format">Format</label>
          <select id="format" bind:value={format}>
            <option value="single">Single event</option>
            <option value="series">Series</option>
          </select>
        </div>
        <div>
          <label for="scoring">Scoring rule</label>
          <select id="scoring" bind:value={scoringRule}>
            {#each SCORERS as s (s)}
              <option value={s}>{s}</option>
            {/each}
          </select>
        </div>
      </div>

      <div class="row two">
        <div>
          <label for="starts">Starts at</label>
          <input id="starts" type="datetime-local" bind:value={startsAt} required />
        </div>
        <div>
          <label for="ends">Ends at</label>
          <input id="ends" type="datetime-local" bind:value={endsAt} required />
        </div>
      </div>

      <fieldset class="optional">
        <legend>Registration window (optional)</legend>
        <div class="row two">
          <div>
            <label for="reg-opens">Opens at</label>
            <input id="reg-opens" type="datetime-local" bind:value={registrationOpensAt} />
          </div>
          <div>
            <label for="reg-closes">Closes at</label>
            <input id="reg-closes" type="datetime-local" bind:value={registrationClosesAt} />
          </div>
        </div>
      </fieldset>

      {#if error}
        <p class="error">{error}</p>
      {/if}

      <button type="submit" disabled={submitting}>
        {submitting ? 'Creating…' : 'Create competition'}
      </button>
      <p class="hint">Created in <code>draft</code> status. You'll add events, categories, and problems on the next page.</p>
    </form>
  {/if}
</main>

<style>
  main {
    max-width: 36rem;
    margin: 0 auto;
    padding: 1.5rem 1rem 4rem;
  }
  h1 {
    font-size: 1.6rem;
    margin: 0.5rem 0 1.5rem;
  }
  form {
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }
  .row label {
    display: block;
    font-size: 0.85rem;
    font-weight: 600;
    color: #334155;
    margin-bottom: 4px;
  }
  legend {
    font-size: 0.85rem;
    font-weight: 600;
    color: #334155;
  }
  .row.two {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }
  input,
  select {
    width: 100%;
    padding: 0.65rem 0.85rem;
    border: 1px solid #cbd5e1;
    border-radius: 8px;
    font-size: 1rem;
    box-sizing: border-box;
  }
  input:focus,
  select:focus {
    outline: 2px solid var(--rw-accent);
    outline-offset: -1px;
    border-color: var(--rw-accent);
  }
  .hint {
    font-size: 0.85rem;
    color: #64748b;
    margin: 4px 0 0;
  }
  .optional {
    border: 1px solid #e2e8f0;
    border-radius: 8px;
    padding: 1rem;
  }
  .optional legend {
    padding: 0 6px;
  }
  button[type='submit'] {
    padding: 0.85rem 1rem;
    background: var(--rw-accent);
    color: #fff;
    border: 0;
    border-radius: 8px;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
  }
  button[type='submit']:disabled {
    background: #fbbf24;
    cursor: not-allowed;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.65rem 0.85rem;
    border-radius: 8px;
    margin: 0;
  }
  .muted {
    color: #94a3b8;
  }
  code {
    background: #f1f5f9;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.9em;
  }
  a {
    color: var(--rw-accent);
  }
</style>
