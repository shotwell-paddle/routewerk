<script lang="ts">
  import type {
    LocationSettingsShape,
    RouteShape,
    RouteWriteShape,
    RouteType,
    WallShape,
  } from '$lib/api/client';

  let {
    initial,
    walls,
    settings = null,
    submitLabel,
    onSubmit,
    onCancel,
    saving = false,
    error = null,
  }: {
    initial?: RouteShape | null;
    walls: WallShape[];
    /**
     * Gym settings — circuits, hold colors, grading defaults. When provided,
     * the form restricts pickers to what the gym actually stocks (matches
     * the HTMX setter form at internal/handler/web/route_form.go). Falls
     * back to a permissive list if null (e.g. the fetch failed or the
     * endpoint isn't readable for this user).
     */
    settings?: LocationSettingsShape | null;
    submitLabel: string;
    onSubmit: (body: RouteWriteShape) => void | Promise<void>;
    onCancel?: () => void;
    saving?: boolean;
    error?: string | null;
  } = $props();

  // svelte-ignore state_referenced_locally
  const seed = initial;

  // Default grade lists — used when the gym hasn't customized v_scale_range
  // / yds_range, or when settings aren't available. Keep in sync with
  // internal/handler/web/route_form.go.
  const DEFAULT_V_GRADES = [
    'VB', 'V0', 'V1', 'V2', 'V3', 'V4', 'V5', 'V6', 'V7',
    'V8', 'V9', 'V10', 'V11', 'V12',
  ];
  const DEFAULT_YDS_GRADES = [
    '5.5', '5.6', '5.7', '5.8-', '5.8', '5.8+',
    '5.9-', '5.9', '5.9+',
    '5.10-', '5.10', '5.10+',
    '5.11-', '5.11', '5.11+',
    '5.12-', '5.12', '5.12+',
    '5.13-', '5.13', '5.13+',
    '5.14-', '5.14',
  ];
  const DEFAULT_CIRCUIT_COLORS = [
    { name: 'Red', hex: '#ef4444' },
    { name: 'Orange', hex: '#f59e0b' },
    { name: 'Yellow', hex: '#eab308' },
    { name: 'Green', hex: '#22c55e' },
    { name: 'Blue', hex: '#3b82f6' },
    { name: 'Purple', hex: '#a855f7' },
    { name: 'Pink', hex: '#ec4899' },
    { name: 'White', hex: '#ffffff' },
    { name: 'Black', hex: '#000000' },
  ];

  // svelte-ignore state_referenced_locally
  let form = $state({
    wall_id: seed?.wall_id ?? '',
    route_type: (seed?.route_type ?? 'boulder') as RouteType,
    // Defaults to V-scale; the snap-to-allowed effect below picks the
    // gym's preference once settings load. Routes use YDS via a separate
    // branch in the template.
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

  function addDays(iso: string, days: number): string {
    if (!iso || !days) return '';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    d.setDate(d.getDate() + days);
    return d.toISOString().slice(0, 10);
  }

  // Gym-defined options (fall back to defaults when settings missing).
  const circuits = $derived(
    settings?.circuits.colors && settings.circuits.colors.length > 0
      ? settings.circuits.colors
      : DEFAULT_CIRCUIT_COLORS,
  );
  const holdColors = $derived(settings?.hold_colors.colors ?? []);
  const stripAgeDays = $derived(settings?.display.default_strip_age_days ?? 0);

  // The boulder grading systems the gym allows climbers to set under.
  // - 'circuits' → only circuit; setters can't fall back to V-scale.
  // - 'v-scale'  → V-scale only.
  // - default    → both, gym hasn't expressed a preference.
  // Routes (not boulders) always allow YDS regardless.
  const allowedBoulderSystems = $derived.by((): string[] => {
    const m = settings?.grading.boulder_method;
    if (m === 'circuits') return ['circuit'];
    if (m === 'v-scale') return ['V-scale'];
    return ['V-scale', 'circuit'];
  });

  // If the gym only allows one boulder system and the current selection
  // isn't it, snap to the allowed one. Keeps a stale seed (or a manual
  // toggle to a hidden option) from leaving the form in an unsubmittable
  // state.
  $effect(() => {
    if (form.route_type !== 'boulder') return;
    if (allowedBoulderSystems.length === 0) return;
    if (!allowedBoulderSystems.includes(form.grading_system)) {
      form.grading_system = allowedBoulderSystems[0];
    }
  });

  // Auto-fill projected_strip_date as date_set + default_strip_age_days
  // unless the user has already typed one in. Only runs on date_set
  // changes; respects an existing seed value.
  let stripUserDirty = $state(!!seed?.projected_strip_date);
  $effect(() => {
    if (stripUserDirty) return;
    if (!stripAgeDays) return;
    const next = addDays(form.date_set, stripAgeDays);
    if (next && next !== form.projected_strip_date) {
      form.projected_strip_date = next;
    }
  });

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
    // For circuit boulders, the user picks a circuit name from the gym's
    // configured set; that pick doubles as both `grade` and `circuit_color`
    // so card layout, leaderboards, and downstream filtering behave the
    // same as the HTMX form (see internal/handler/web/setter_routes.go).
    const circuitColor =
      form.grading_system === 'circuit'
        ? form.grade || null
        : form.circuit_color || null;

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
      circuit_color: circuitColor,
    };
  }

  function handleSubmit(e: Event) {
    e.preventDefault();
    localError = null;
    const body = buildBody();
    if (!body) return;
    onSubmit(body);
  }

  // Pick the right grade list for the selected scale. Honor any custom
  // ranges the gym has set; fall back to the full default list otherwise.
  const gradeOptions = $derived.by((): string[] => {
    if (form.grading_system === 'YDS') {
      return settings?.grading.yds_range && settings.grading.yds_range.length > 0
        ? settings.grading.yds_range
        : DEFAULT_YDS_GRADES;
    }
    if (form.grading_system === 'circuit') {
      // Circuit "grade" is the circuit color (mirrors the HTMX form).
      return circuits.map((c) => c.name);
    }
    return settings?.grading.v_scale_range && settings.grading.v_scale_range.length > 0
      ? settings.grading.v_scale_range
      : DEFAULT_V_GRADES;
  });
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
        {#if form.route_type === 'boulder'}
          {#if allowedBoulderSystems.includes('V-scale')}
            <option value="V-scale">V-scale</option>
          {/if}
          {#if allowedBoulderSystems.includes('circuit')}
            <option value="circuit">Circuit</option>
          {/if}
        {:else}
          <option value="YDS">YDS</option>
        {/if}
      </select>
    </div>
    <div class="field">
      <label for="r-grade">
        {form.grading_system === 'circuit' ? 'Circuit *' : 'Grade *'}
      </label>
      <select id="r-grade" bind:value={form.grade}>
        <option value="">
          {form.grading_system === 'circuit' ? 'Pick a circuit…' : 'Pick a grade…'}
        </option>
        {#each gradeOptions as g}
          <option value={g}>{g}</option>
        {/each}
      </select>
    </div>
  </div>

  <div class="row">
    <div class="field">
      <label for="r-name">Name</label>
      <input id="r-name" bind:value={form.name} placeholder="optional" />
    </div>
    <div class="field">
      <label for="r-color">Hold color *</label>
      <div class="color-input">
        {#if holdColors.length > 0}
          <select id="r-color" bind:value={form.color}>
            <option value="">Pick a color…</option>
            {#each holdColors as c (c.name)}
              <option value={c.hex}>{c.name}</option>
            {/each}
          </select>
          <span class="color-swatch" style="background:{form.color || '#e8e6e1'}"></span>
        {:else}
          <input id="r-color" type="color" bind:value={form.color} />
          <span class="color-hex">{form.color || '#e8e6e1'}</span>
        {/if}
      </div>
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
      <label for="r-strip">
        Projected strip date
        {#if stripAgeDays > 0 && !stripUserDirty}
          <span class="hint-tag">auto from gym default</span>
        {/if}
      </label>
      <input
        id="r-strip"
        type="date"
        bind:value={form.projected_strip_date}
        oninput={() => (stripUserDirty = true)} />
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
  .color-input {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 8px;
    align-items: center;
  }
  .color-input input[type='color'] {
    height: 38px;
    padding: 2px;
    cursor: pointer;
  }
  .color-swatch {
    display: inline-block;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .color-hex {
    color: var(--rw-text-muted);
    font-size: 0.85rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .hint-tag {
    margin-left: 6px;
    color: var(--rw-text-faint);
    font-size: 0.7rem;
    font-weight: 500;
    text-transform: none;
    letter-spacing: 0;
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
