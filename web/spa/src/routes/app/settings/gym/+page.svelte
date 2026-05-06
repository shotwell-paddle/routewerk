<script lang="ts">
  import {
    getLocationSettings,
    updateLocationSettings,
    ApiClientError,
    type LocationSettingsShape,
    type CircuitColor,
    type HoldColor,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';

  let settings = $state<LocationSettingsShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let saving = $state(false);
  let saveOk = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());
  const canEdit = $derived(roleRankAt(locId) >= 4);

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    saveOk = null;
    getLocationSettings(locId)
      .then((s) => {
        if (!cancelled) settings = s;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load settings.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  // ── Circuit color editing ────────────────────────────────

  let newCircuit = $state({ name: '', hex: '#ff5500', sort_order: 0 });

  function addCircuit() {
    if (!settings) return;
    const trimmed = newCircuit.name.trim();
    if (!trimmed) return;
    settings.circuits.colors = [
      ...settings.circuits.colors,
      {
        name: trimmed,
        hex: newCircuit.hex,
        sort_order: settings.circuits.colors.length,
      },
    ];
    newCircuit = { name: '', hex: '#ff5500', sort_order: 0 };
  }

  function removeCircuit(idx: number) {
    if (!settings) return;
    settings.circuits.colors = settings.circuits.colors.filter((_, i) => i !== idx);
  }

  function moveCircuit(idx: number, dir: -1 | 1) {
    if (!settings) return;
    const arr = settings.circuits.colors.slice();
    const target = idx + dir;
    if (target < 0 || target >= arr.length) return;
    [arr[idx], arr[target]] = [arr[target], arr[idx]];
    settings.circuits.colors = arr.map((c, i) => ({ ...c, sort_order: i }));
  }

  // ── Hold color editing ───────────────────────────────────

  let newHoldColor = $state({ name: '', hex: '#ff5500' });

  function addHoldColor() {
    if (!settings) return;
    const trimmed = newHoldColor.name.trim();
    if (!trimmed) return;
    settings.hold_colors.colors = [
      ...settings.hold_colors.colors,
      { name: trimmed, hex: newHoldColor.hex },
    ];
    newHoldColor = { name: '', hex: '#ff5500' };
  }

  function removeHoldColor(idx: number) {
    if (!settings) return;
    settings.hold_colors.colors = settings.hold_colors.colors.filter((_, i) => i !== idx);
  }

  // ── Save ──────────────────────────────────────────────────

  async function save(e: Event) {
    e.preventDefault();
    if (!locId || !settings) return;
    saving = true;
    error = null;
    saveOk = null;
    try {
      settings = await updateLocationSettings(locId, settings);
      saveOk = 'Saved.';
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Gym settings — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/settings">← Settings</a>
  <h1>Gym settings</h1>
  <p class="lede">
    Circuits, hold colors, grading defaults, and what climbers see on route cards.
  </p>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if !canEdit}
    <p class="muted">
      Only gym managers and above can edit gym settings. Switch locations or ask
      a manager for access.
    </p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else if error && !settings}
    <p class="error">{error}</p>
  {:else if settings}
    <form onsubmit={save}>
      <section class="card">
        <h2>Circuits</h2>
        <p class="muted small">
          Color-coded difficulty bands. Climbers see these on circuit boulders.
          Drag the up/down arrows to reorder; the order shown here is the order
          climbers see.
        </p>
        <ul class="color-list">
          {#each settings.circuits.colors as c, i (i + ':' + c.name)}
            <li class="color-row">
              <span class="swatch" style="background:{c.hex}"></span>
              <input bind:value={c.name} class="name-field" />
              <input type="color" bind:value={c.hex} class="hex-field" />
              <button type="button" class="icon" onclick={() => moveCircuit(i, -1)} disabled={i === 0} aria-label="Move up">↑</button>
              <button type="button" class="icon" onclick={() => moveCircuit(i, 1)} disabled={i === settings.circuits.colors.length - 1} aria-label="Move down">↓</button>
              <button type="button" class="icon danger" onclick={() => removeCircuit(i)} aria-label="Remove">✕</button>
            </li>
          {/each}
        </ul>
        <div class="add-row">
          <input bind:value={newCircuit.name} placeholder="New circuit (e.g. Crimson)" />
          <input type="color" bind:value={newCircuit.hex} />
          <button type="button" onclick={addCircuit} disabled={!newCircuit.name.trim()}>Add</button>
        </div>
      </section>

      <section class="card">
        <h2>Hold colors</h2>
        <p class="muted small">
          What hold colors are stocked at this gym. Used as the picker on the
          route create form and the color chip on cards.
        </p>
        <ul class="color-list">
          {#each settings.hold_colors.colors as c, i (i + ':' + c.name)}
            <li class="color-row">
              <span class="swatch" style="background:{c.hex}"></span>
              <input bind:value={c.name} class="name-field" />
              <input type="color" bind:value={c.hex} class="hex-field" />
              <button type="button" class="icon danger" onclick={() => removeHoldColor(i)} aria-label="Remove">✕</button>
            </li>
          {/each}
        </ul>
        <div class="add-row">
          <input bind:value={newHoldColor.name} placeholder="New color (e.g. Teal)" />
          <input type="color" bind:value={newHoldColor.hex} />
          <button type="button" onclick={addHoldColor} disabled={!newHoldColor.name.trim()}>Add</button>
        </div>
      </section>

      <section class="card">
        <h2>Grading</h2>
        <div class="row">
          <label>
            <span>Boulder method</span>
            <select bind:value={settings.grading.boulder_method}>
              <option value="v_scale">V-scale</option>
              <option value="circuit">Circuit</option>
            </select>
          </label>
          <label>
            <span>Route grade format</span>
            <select bind:value={settings.grading.route_grade_format}>
              <option value="yds">YDS</option>
              <option value="french">French</option>
            </select>
          </label>
        </div>
        <label class="check">
          <input type="checkbox" bind:checked={settings.grading.show_grades_on_circuit} />
          Show V-scale grade on circuit boulders
        </label>
      </section>

      <section class="card">
        <h2>Display</h2>
        <label class="check">
          <input type="checkbox" bind:checked={settings.display.show_setter_name} />
          Show setter name on route cards
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={settings.display.show_route_age} />
          Show route age on cards
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={settings.display.show_difficulty_consensus} />
          Show difficulty consensus
        </label>
        <label class="inline">
          <span>Default strip age (days)</span>
          <input type="number" min="0" step="1" bind:value={settings.display.default_strip_age_days} />
        </label>
      </section>

      <section class="card">
        <h2>Sessions</h2>
        <label class="check">
          <input type="checkbox" bind:checked={settings.sessions.default_playbook_enabled} />
          New sessions get the default playbook checklist
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={settings.sessions.require_route_photo} />
          Require route photo before publishing
        </label>
      </section>

      {#if error}<p class="error">{error}</p>{/if}
      {#if saveOk}<p class="ok">{saveOk}</p>{/if}

      <div class="actions">
        <button class="primary" type="submit" disabled={saving}>
          {saving ? 'Saving…' : 'Save settings'}
        </button>
      </div>
    </form>
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
  .lede {
    color: var(--rw-text-muted);
    margin: 0 0 1.25rem;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .card h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.6rem;
  }
  .small {
    font-size: 0.85rem;
    margin: 0 0 0.85rem;
  }
  .color-list {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .color-row {
    display: grid;
    grid-template-columns: 24px 1fr auto auto auto auto;
    align-items: center;
    gap: 8px;
  }
  .swatch {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .name-field,
  .add-row input:not([type='color']) {
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.9rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .hex-field,
  .add-row input[type='color'] {
    width: 36px;
    height: 30px;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    padding: 0;
    cursor: pointer;
  }
  .add-row {
    display: flex;
    align-items: center;
    gap: 8px;
    border-top: 1px dashed var(--rw-border);
    padding-top: 0.85rem;
  }
  .add-row input {
    flex: 1;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.85rem;
    margin-bottom: 0.75rem;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.85rem;
    color: var(--rw-text-muted);
    font-weight: 600;
  }
  label.check {
    flex-direction: row;
    align-items: center;
    gap: 8px;
    color: var(--rw-text);
    font-weight: 500;
    margin-bottom: 0.5rem;
  }
  label.inline {
    flex-direction: row;
    align-items: center;
    gap: 12px;
    margin-top: 0.5rem;
  }
  label.inline input {
    width: 6rem;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
  }
  select {
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .actions {
    display: flex;
    gap: 8px;
    margin-top: 1rem;
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
  button.icon {
    padding: 0.25rem 0.5rem;
    font-size: 0.85rem;
  }
  button.icon.danger {
    color: #b91c1c;
    border-color: #fecaca;
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
