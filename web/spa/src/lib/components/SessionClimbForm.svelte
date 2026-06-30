<script lang="ts">
  import type {
    LocationSettingsShape,
    SessionRouteDetailShape,
    SessionRouteWriteShape,
    TeamMemberShape,
    WallShape,
  } from '$lib/api/client';

  let {
    wall,
    settings = null,
    setters = [],
    defaultSetterId = '',
    initial = null,
    submitLabel = 'Add climb',
    onSubmit,
    onCancel,
    saving = false,
    error = null,
  }: {
    /** The wall this section is for — fixes route_type + drives grading. */
    wall: WallShape;
    /** Gym settings: circuits, hold colors, grading ranges + boulder method. */
    settings?: LocationSettingsShape | null;
    setters?: TeamMemberShape[];
    /** Pre-select this setter (the wall section's assigned setter). */
    defaultSetterId?: string;
    /** When set, the form edits an existing draft climb instead of adding one. */
    initial?: SessionRouteDetailShape | null;
    submitLabel?: string;
    onSubmit: (body: SessionRouteWriteShape) => void | Promise<void>;
    onCancel?: () => void;
    saving?: boolean;
    error?: string | null;
  } = $props();

  // Keep these in sync with RouteForm.svelte / internal/handler/web/route_form.go.
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

  const isRopeWall = $derived(wall.wall_type === 'route');

  // The gym's preferred default boulder system. The backend stores
  // boulder_method as 'v_scale' | 'circuit' | 'both'; only 'circuit'
  // defaults a new climb to circuits, everything else defaults to V-scale.
  // This sets the DEFAULT only — the setter can always switch to the other
  // system below (we never lock them to one). Maps to the grading_system
  // values routes are stored under ('V-scale' | 'circuit').
  const preferredBoulderSystem = $derived(
    settings?.grading.boulder_method === 'circuit' ? 'circuit' : 'V-scale',
  );

  function initialGradingSystem(): string {
    if (initial?.grading_system) return initial.grading_system;
    if (wall.wall_type === 'route') return 'YDS';
    return settings?.grading.boulder_method === 'circuit' ? 'circuit' : 'V-scale';
  }

  // svelte-ignore state_referenced_locally
  let form = $state({
    grading_system: initialGradingSystem(),
    grade: initial?.grade ?? '',
    color: initial?.color ?? '',
    name: initial?.name ?? '',
    setter_id: initial?.setter_id ?? defaultSetterId ?? '',
  });

  let localError = $state<string | null>(null);

  const circuits = $derived(
    settings?.circuits.colors && settings.circuits.colors.length > 0
      ? settings.circuits.colors
      : DEFAULT_CIRCUIT_COLORS,
  );
  const holdColors = $derived(settings?.hold_colors.colors ?? []);

  // Rope walls are always YDS. Boulder walls must be V-scale or circuit;
  // snap a stale/invalid value (e.g. a leftover 'YDS' from a wall-type
  // switch) to the gym's preferred default. The setter's own V-scale⇄
  // circuit choice is preserved.
  $effect(() => {
    if (isRopeWall) {
      if (form.grading_system !== 'YDS') form.grading_system = 'YDS';
      return;
    }
    if (form.grading_system !== 'V-scale' && form.grading_system !== 'circuit') {
      form.grading_system = preferredBoulderSystem;
    }
  });

  const gradeOptions = $derived.by((): string[] => {
    if (form.grading_system === 'YDS') {
      return settings?.grading.yds_range && settings.grading.yds_range.length > 0
        ? settings.grading.yds_range
        : DEFAULT_YDS_GRADES;
    }
    if (form.grading_system === 'circuit') {
      return circuits.map((c) => c.name);
    }
    return settings?.grading.v_scale_range && settings.grading.v_scale_range.length > 0
      ? settings.grading.v_scale_range
      : DEFAULT_V_GRADES;
  });

  const isCircuit = $derived(form.grading_system === 'circuit');

  function buildBody(): SessionRouteWriteShape | null {
    if (!form.grade) {
      localError = isCircuit ? 'Pick a circuit.' : 'Pick a grade.';
      return null;
    }
    if (!form.color) {
      localError = 'Pick a hold color.';
      return null;
    }
    return {
      wall_id: wall.id,
      route_type: isRopeWall ? 'route' : 'boulder',
      grading_system: form.grading_system,
      grade: form.grade,
      color: form.color,
      // For circuits the picked color name doubles as grade + circuit_color
      // (matches RouteForm + the HTMX setter form).
      circuit_color: isCircuit ? form.grade : null,
      name: form.name.trim() || null,
      setter_id: form.setter_id || null,
    };
  }

  function handleSubmit(e: Event) {
    e.preventDefault();
    localError = null;
    const body = buildBody();
    if (!body) return;
    onSubmit(body);
  }
</script>

<form class="climb-form" onsubmit={handleSubmit}>
  <div class="grid">
    {#if !isRopeWall}
      <!-- Setters can always choose either system; the dropdown just
           defaults to the gym's boulder_method preference. -->
      <label class="field">
        <span>Style</span>
        <select bind:value={form.grading_system}>
          <option value="V-scale">V-scale</option>
          <option value="circuit">Circuit</option>
        </select>
      </label>
    {/if}

    <label class="field">
      <span>{isCircuit ? 'Circuit *' : 'Grade *'}</span>
      <select bind:value={form.grade}>
        <option value="">{isCircuit ? 'Pick a circuit…' : 'Pick a grade…'}</option>
        {#each gradeOptions as g}
          <option value={g}>{g}</option>
        {/each}
      </select>
    </label>

    <label class="field">
      <span>Hold color *</span>
      <div class="color-input">
        {#if holdColors.length > 0}
          <select bind:value={form.color}>
            <option value="">Pick…</option>
            {#each holdColors as c (c.name)}
              <option value={c.hex}>{c.name}</option>
            {/each}
          </select>
        {:else}
          <input type="color" bind:value={form.color} />
        {/if}
        <span class="swatch" style="background:{form.color || '#e8e6e1'}"></span>
      </div>
    </label>
  </div>

  <div class="grid">
    <label class="field">
      <span>Name</span>
      <input bind:value={form.name} placeholder="optional" />
    </label>
    <label class="field">
      <span>Setter</span>
      <select bind:value={form.setter_id}>
        <option value="">Unassigned</option>
        {#each setters as s (s.user_id)}
          <option value={s.user_id}>{s.display_name}</option>
        {/each}
      </select>
    </label>
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
  .climb-form {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }
  .grid {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--rw-text-muted);
    flex: 1 1 8rem;
    min-width: 7rem;
  }
  .field input,
  .field select {
    padding: 0.45rem 0.55rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.9rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-family: inherit;
    box-sizing: border-box;
  }
  .field input:focus,
  .field select:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  .color-input {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 6px;
    align-items: center;
  }
  .color-input input[type='color'] {
    height: 36px;
    padding: 2px;
    cursor: pointer;
  }
  .swatch {
    display: inline-block;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
    flex-shrink: 0;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  button {
    cursor: pointer;
    padding: 0.45rem 0.85rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.85rem;
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
    padding: 0.45rem 0.65rem;
    border-radius: 6px;
    font-size: 0.85rem;
    margin: 0;
  }
</style>
