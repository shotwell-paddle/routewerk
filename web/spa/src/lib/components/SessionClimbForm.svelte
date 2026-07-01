<script lang="ts">
  import type {
    LocationSettingsShape,
    SessionRouteDetailShape,
    SessionRouteWriteShape,
    TeamMemberShape,
    WallShape,
  } from '$lib/api/client';
  import {
    DEFAULT_V_GRADES,
    DEFAULT_YDS_GRADES,
    DEFAULT_CIRCUIT_COLORS,
  } from '$lib/grades';

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

  const isRopeWall = $derived(wall.wall_type === 'route');

  // grading_system values MUST match the Postgres `grading_system` enum
  // exactly: 'v_scale' | 'yds' | 'circuit' (lowercase). Sending 'V-scale'
  // or 'YDS' makes the INSERT fail the enum constraint — which is why
  // rope (YDS) and V-scale boulder climbs silently failed to save.

  // The gym's preferred default boulder system. The backend stores
  // boulder_method as 'v_scale' | 'circuit' | 'both'; only 'circuit'
  // defaults a new climb to circuits, everything else defaults to v_scale.
  // This sets the DEFAULT only — the setter can always switch below.
  const preferredBoulderSystem = $derived(
    settings?.grading.boulder_method === 'circuit' ? 'circuit' : 'v_scale',
  );

  function initialGradingSystem(): string {
    if (initial?.grading_system) return initial.grading_system;
    if (wall.wall_type === 'route') return 'yds';
    return settings?.grading.boulder_method === 'circuit' ? 'circuit' : 'v_scale';
  }

  // svelte-ignore state_referenced_locally
  let form = $state({
    grading_system: initialGradingSystem(),
    grade: initial?.grade ?? '',
    color: initial?.color ?? '',
    name: initial?.name ?? '',
    setter_id: initial?.setter_id ?? defaultSetterId ?? '',
    // Kids rope route: sets the circuit_color='kids' marker the dashboard
    // buckets on. Boulders get their kids marker from the kids circuit.
    kids: (initial?.circuit_color ?? '').toLowerCase() === 'kids',
  });

  let localError = $state<string | null>(null);

  const circuits = $derived(
    settings?.circuits.colors && settings.circuits.colors.length > 0
      ? settings.circuits.colors
      : DEFAULT_CIRCUIT_COLORS,
  );
  const holdColors = $derived(settings?.hold_colors.colors ?? []);

  // Rope walls are always YDS. Boulder walls must be v_scale or circuit;
  // snap a stale/invalid value (e.g. a leftover 'yds' from a wall-type
  // switch) to the gym's preferred default. The setter's own v_scale⇄
  // circuit choice is preserved.
  $effect(() => {
    if (isRopeWall) {
      if (form.grading_system !== 'yds') form.grading_system = 'yds';
      return;
    }
    if (form.grading_system !== 'v_scale' && form.grading_system !== 'circuit') {
      form.grading_system = preferredBoulderSystem;
    }
  });

  const gradeOptions = $derived.by((): string[] => {
    if (form.grading_system === 'yds') {
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

  // Normalize a hex for comparison: lowercase + expand 3-digit shorthand.
  // A stored route color can differ in case/shorthand from the palette hex
  // (the backend matches case-insensitively too — see route.go holdColorNames),
  // so a strict === would fail to highlight the right swatch in edit mode.
  function normHex(h: string | null | undefined): string {
    let s = (h ?? '').trim().toLowerCase();
    if (/^#[0-9a-f]{3}$/.test(s)) {
      s = '#' + s[1] + s[1] + s[2] + s[2] + s[3] + s[3];
    }
    return s;
  }

  function isPickedColor(hex: string): boolean {
    return normHex(form.color) === normHex(hex);
  }

  // Name of the currently-picked hold color (for the readout next to the
  // swatch grid). Empty when nothing is selected or the hex isn't in the
  // gym palette.
  const selectedColorName = $derived(
    holdColors.find((c) => isPickedColor(c.hex))?.name ?? '',
  );

  // Switching boulder style clears the grade so a stale V-grade can't ride
  // along as an invalid circuit name (or vice versa).
  function selectStyle(sys: string) {
    if (form.grading_system === sys) return;
    form.grading_system = sys;
    form.grade = '';
  }

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
      circuit_color: isCircuit ? form.grade : isRopeWall && form.kids ? 'kids' : null,
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
  {#if !isRopeWall}
    <!-- Setters can always choose either system; it just defaults to the
         gym's boulder_method preference. -->
    <div class="field">
      <span>Style</span>
      <div class="seg" role="group" aria-label="Grading style">
        <button type="button" class="seg-btn" class:on={form.grading_system === 'v_scale'}
                aria-pressed={form.grading_system === 'v_scale'}
                onclick={() => selectStyle('v_scale')}>V-scale</button>
        <button type="button" class="seg-btn" class:on={form.grading_system === 'circuit'}
                aria-pressed={form.grading_system === 'circuit'}
                onclick={() => selectStyle('circuit')}>Circuit</button>
      </div>
    </div>
  {/if}

  {#if isRopeWall}
    <div class="field">
      <span>Category</span>
      <div class="seg" role="group" aria-label="Route category">
        <button type="button" class="seg-btn" class:on={!form.kids}
                aria-pressed={!form.kids} onclick={() => (form.kids = false)}>Standard</button>
        <button type="button" class="seg-btn" class:on={form.kids}
                aria-pressed={form.kids} onclick={() => (form.kids = true)}>Kids</button>
      </div>
    </div>
  {/if}

  <div class="field">
    <span>{isCircuit ? 'Circuit *' : 'Grade *'}</span>
    <div class="chips">
      {#each gradeOptions as g}
        <button type="button" class="chip" class:on={form.grade === g}
                aria-pressed={form.grade === g}
                onclick={() => (form.grade = g)}>{g}</button>
      {/each}
    </div>
  </div>

  <div class="field">
    <span>Hold color *{#if selectedColorName}<span class="picked"> · {selectedColorName}</span>{/if}</span>
    {#if holdColors.length > 0}
      <div class="swatches">
        {#each holdColors as c (c.name)}
          <button type="button" class="sw" class:on={isPickedColor(c.hex)}
                  aria-pressed={isPickedColor(c.hex)}
                  style="background:{c.hex}" title={c.name} aria-label={c.name}
                  onclick={() => (form.color = c.hex)}></button>
        {/each}
      </div>
    {:else}
      <div class="color-input">
        <input type="color" bind:value={form.color} />
        <span class="swatch" style="background:{form.color || '#e8e6e1'}"></span>
      </div>
    {/if}
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
  .picked {
    font-weight: 500;
    color: var(--rw-text-muted);
  }
  .seg {
    display: inline-flex;
    align-self: flex-start;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    overflow: hidden;
  }
  .seg-btn {
    border: none;
    border-radius: 0;
    padding: 0.4rem 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text-muted);
    font-size: 0.82rem;
    font-weight: 600;
  }
  .seg-btn + .seg-btn {
    border-left: 1px solid var(--rw-border-strong);
  }
  .seg-btn.on {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .chip {
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    padding: 0.35rem 0.7rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.85rem;
    font-weight: 600;
    min-width: 2.5rem;
  }
  .chip.on {
    border-color: var(--rw-accent);
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
  }
  .swatches {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .sw {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
    padding: 0;
  }
  .sw.on {
    box-shadow:
      0 0 0 2px var(--rw-surface),
      0 0 0 4px var(--rw-accent);
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
