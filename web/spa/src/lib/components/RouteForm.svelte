<script lang="ts">
  import type {
    RouteShape,
    RouteWriteShape,
    RouteType,
    WallShape,
  } from '$lib/api/client';

  let {
    initial,
    walls,
    submitLabel,
    onSubmit,
    onCancel,
    saving = false,
    error = null,
  }: {
    initial?: RouteShape | null;
    walls: WallShape[];
    submitLabel: string;
    onSubmit: (body: RouteWriteShape) => void | Promise<void>;
    onCancel?: () => void;
    saving?: boolean;
    error?: string | null;
  } = $props();

  // svelte-ignore state_referenced_locally
  const seed = initial;

  // Mirror the HTMX form's grade pick lists. Keep the source-of-truth
  // copy here in sync with internal/handler/web/route_form.go — should
  // pull from a JSON endpoint eventually.
  const V_GRADES = [
    'VB', 'V0', 'V1', 'V2', 'V3', 'V4', 'V5', 'V6', 'V7',
    'V8', 'V9', 'V10', 'V11', 'V12',
  ];
  const YDS_GRADES = [
    '5.5', '5.6', '5.7', '5.8-', '5.8', '5.8+',
    '5.9-', '5.9', '5.9+',
    '5.10-', '5.10', '5.10+',
    '5.11-', '5.11', '5.11+',
    '5.12-', '5.12', '5.12+',
    '5.13-', '5.13', '5.13+',
    '5.14-', '5.14',
  ];
  const CIRCUIT_COLORS = [
    'red', 'orange', 'yellow', 'green', 'blue', 'purple', 'pink', 'white', 'black',
  ];

  let form = $state({
    wall_id: seed?.wall_id ?? '',
    route_type: (seed?.route_type ?? 'boulder') as RouteType,
    grading_system: seed?.grading_system ?? 'V-scale',
    grade: seed?.grade ?? '',
    name: seed?.name ?? '',
    color: seed?.color ?? '',
    description: seed?.description ?? '',
    photo_url: seed?.photo_url ?? '',
    date_set: isoDate(seed?.date_set ?? new Date().toISOString()),
    projected_strip_date: isoDate(seed?.projected_strip_date ?? null),
    circuit_color: seed?.circuit_color ?? '',
  });

  let localError = $state<string | null>(null);

  function isoDate(input: string | null | undefined): string {
    if (!input) return '';
    return input.slice(0, 10);
  }

  function buildBody(): RouteWriteShape | null {
    if (!form.wall_id) {
      localError = 'Wall is required.';
      return null;
    }
    if (!form.grade) {
      localError = 'Grade is required.';
      return null;
    }
    if (!form.color) {
      localError = 'Hold color is required.';
      return null;
    }
    return {
      wall_id: form.wall_id,
      route_type: form.route_type,
      grading_system: form.grading_system,
      grade: form.grade,
      color: form.color,
      name: form.name.trim() || null,
      description: form.description.trim() || null,
      photo_url: form.photo_url.trim() || null,
      date_set: form.date_set || null,
      projected_strip_date: form.projected_strip_date || null,
      circuit_color: form.circuit_color || null,
    };
  }

  function handleSubmit(e: Event) {
    e.preventDefault();
    localError = null;
    const body = buildBody();
    if (!body) return;
    onSubmit(body);
  }

  // Pick the right grade list for the selected scale.
  const gradeOptions = $derived(
    form.grading_system === 'YDS' ? YDS_GRADES : form.grading_system === 'circuit' ? [] : V_GRADES,
  );
</script>

<form class="form" onsubmit={handleSubmit}>
  <div class="row">
    <div class="field">
      <label for="r-wall">Wall *</label>
      <select id="r-wall" bind:value={form.wall_id}>
        <option value="">Pick a wall…</option>
        {#each walls as w (w.id)}
          <option value={w.id}>{w.name}</option>
        {/each}
      </select>
    </div>
    <div class="field">
      <label for="r-type">Type *</label>
      <select id="r-type" bind:value={form.route_type}>
        <option value="boulder">Boulder</option>
        <option value="route">Route</option>
      </select>
    </div>
  </div>

  <div class="row">
    <div class="field">
      <label for="r-system">Grading system</label>
      <select id="r-system" bind:value={form.grading_system}>
        <option value="V-scale">V-scale</option>
        <option value="YDS">YDS</option>
        <option value="circuit">Circuit</option>
      </select>
    </div>
    <div class="field">
      <label for="r-grade">Grade *</label>
      {#if gradeOptions.length > 0}
        <select id="r-grade" bind:value={form.grade}>
          <option value="">Pick a grade…</option>
          {#each gradeOptions as g}
            <option value={g}>{g}</option>
          {/each}
        </select>
      {:else}
        <input
          id="r-grade"
          bind:value={form.grade}
          placeholder="Free text for circuit boulders" />
      {/if}
    </div>
  </div>

  {#if form.grading_system === 'circuit'}
    <div class="row">
      <div class="field">
        <label for="r-circuit">Circuit color</label>
        <select id="r-circuit" bind:value={form.circuit_color}>
          <option value="">—</option>
          {#each CIRCUIT_COLORS as c}
            <option value={c}>{c}</option>
          {/each}
        </select>
      </div>
      <div class="field"></div>
    </div>
  {/if}

  <div class="row">
    <div class="field">
      <label for="r-name">Name</label>
      <input id="r-name" bind:value={form.name} placeholder="optional" />
    </div>
    <div class="field">
      <label for="r-color">Hold color *</label>
      <input
        id="r-color"
        bind:value={form.color}
        placeholder="hex like #ff5500 or named like 'red'" />
    </div>
  </div>

  <div class="field">
    <label for="r-desc">Description / beta</label>
    <textarea id="r-desc" bind:value={form.description} rows="3"></textarea>
  </div>

  <div class="field">
    <label for="r-photo">Photo URL</label>
    <input id="r-photo" bind:value={form.photo_url} placeholder="https://…" />
  </div>

  <div class="row">
    <div class="field">
      <label for="r-set">Set date</label>
      <input id="r-set" type="date" bind:value={form.date_set} />
    </div>
    <div class="field">
      <label for="r-strip">Projected strip date</label>
      <input id="r-strip" type="date" bind:value={form.projected_strip_date} />
    </div>
  </div>

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
    max-width: 40rem;
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
