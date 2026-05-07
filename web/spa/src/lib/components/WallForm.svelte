<script lang="ts">
  import type { WallShape, WallWriteShape, WallType } from '$lib/api/client';

  let {
    initial,
    submitLabel,
    onSubmit,
    onCancel,
    saving = false,
    error = null,
  }: {
    initial?: WallShape | null;
    submitLabel: string;
    onSubmit: (body: WallWriteShape) => void | Promise<void>;
    onCancel?: () => void;
    saving?: boolean;
    error?: string | null;
  } = $props();

  // Snapshot the seed values once at mount — `initial` is a one-shot seed,
  // not a live source. The form state is then user-owned.
  // svelte-ignore state_referenced_locally
  const seed = initial;
  let form = $state({
    name: seed?.name ?? '',
    wall_type: (seed?.wall_type ?? 'boulder') as WallType,
    angle: seed?.angle ?? '',
    height_meters: seed?.height_meters != null ? String(seed.height_meters) : '',
    num_anchors: seed?.num_anchors != null ? String(seed.num_anchors) : '',
    surface_type: seed?.surface_type ?? '',
    sort_order: String(seed?.sort_order ?? 0),
  });

  let localError = $state<string | null>(null);

  function buildBody(): WallWriteShape | null {
    if (!form.name.trim()) {
      localError = 'Name is required.';
      return null;
    }
    return {
      name: form.name.trim(),
      wall_type: form.wall_type,
      angle: form.angle.trim() || null,
      height_meters: form.height_meters.trim() ? parseFloat(form.height_meters) : null,
      num_anchors: form.num_anchors.trim() ? parseInt(form.num_anchors, 10) : null,
      surface_type: form.surface_type.trim() || null,
      sort_order: parseInt(form.sort_order, 10) || 0,
    };
  }

  function handleSubmit(e: Event) {
    e.preventDefault();
    localError = null;
    const body = buildBody();
    if (!body) return;
    onSubmit(body);
  }

  // Pick-list of common angle descriptors. Free-text override allowed —
  // some walls have unusual descriptions (e.g. "30° tilt at the lip").
  const ANGLES = ['Slab', 'Vertical', 'Overhang', 'Roof', 'Arête', 'Corner'];
  const SURFACES = ['Wood', 'Plywood', 'Concrete', 'Plastic', 'Other'];
</script>

<form class="form" onsubmit={handleSubmit}>
  <div class="row">
    <div class="field">
      <label for="wall-name">Name *</label>
      <input id="wall-name" bind:value={form.name} placeholder="Cave, Slab Wall, Comp Wall…" />
    </div>
    <div class="field">
      <label for="wall-type">Type *</label>
      <select id="wall-type" bind:value={form.wall_type}>
        <option value="boulder">Boulder</option>
        <option value="route">Route</option>
      </select>
    </div>
  </div>

  <div class="row">
    <div class="field">
      <label for="wall-angle">Angle</label>
      <input
        id="wall-angle"
        list="wall-angle-options"
        bind:value={form.angle}
        placeholder="Vertical, 30° overhang…" />
      <datalist id="wall-angle-options">
        {#each ANGLES as a}<option value={a}></option>{/each}
      </datalist>
    </div>
    <div class="field">
      <label for="wall-surface">Surface</label>
      <input
        id="wall-surface"
        list="wall-surface-options"
        bind:value={form.surface_type}
        placeholder="Wood, plastic…" />
      <datalist id="wall-surface-options">
        {#each SURFACES as s}<option value={s}></option>{/each}
      </datalist>
    </div>
  </div>

  <div class="row">
    <div class="field">
      <label for="wall-height">Height (m)</label>
      <input
        id="wall-height"
        type="number"
        step="0.1"
        min="0"
        bind:value={form.height_meters}
        placeholder="e.g. 4.5" />
    </div>
    {#if form.wall_type === 'route'}
      <div class="field">
        <label for="wall-anchors">Anchors</label>
        <input
          id="wall-anchors"
          type="number"
          min="0"
          step="1"
          bind:value={form.num_anchors}
          placeholder="e.g. 12" />
      </div>
    {:else}
      <div class="field">
        <label for="wall-sort">Sort order</label>
        <input id="wall-sort" type="number" step="1" bind:value={form.sort_order} />
      </div>
    {/if}
  </div>

  {#if form.wall_type === 'route'}
    <div class="row">
      <div class="field">
        <label for="wall-sort-r">Sort order</label>
        <input id="wall-sort-r" type="number" step="1" bind:value={form.sort_order} />
      </div>
      <div class="field"></div>
    </div>
  {/if}

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
    max-width: 36rem;
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
  .field select {
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    box-sizing: border-box;
  }
  .field input:focus,
  .field select:focus {
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
