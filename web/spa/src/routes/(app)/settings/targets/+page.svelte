<script lang="ts">
  import { goto } from '$app/navigation';
  import {
    listDistributionTargets,
    replaceDistributionTargets,
    listRoutes,
    ApiClientError,
    type DistributionTarget,
    type DistributionTargetType,
    type RouteShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { authState, roleRankAt } from '$lib/stores/auth.svelte';

  // Distribution targets editor — head_setter+ configures "what the
  // gym's mix should look like" so the dashboard charts can overlay
  // actual vs target. Per-(location, route_type, grade) bucket.

  const locId = $derived(effectiveLocationId());
  const canEdit = $derived(roleRankAt(locId) >= 3);

  type Row = {
    key: string; // local-only identifier so Svelte's keyed each tracks correctly
    route_type: DistributionTargetType;
    grade: string;
    target_count: number;
  };

  let rows = $state<Row[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let saving = $state(false);
  let savedFlash = $state<string | null>(null);

  // Active routes drive the "Add from observed grades" suggestions so
  // a head setter doesn't have to remember every grade their routes use.
  let observedGrades = $state<{ route_type: DistributionTargetType; grade: string }[]>([]);

  // Page-level role gate. Same pattern as the playbook editor.
  $effect(() => {
    if (!authState().loaded || !authState().me) return;
    if (!canEdit) goto('/settings');
  });

  let nextKey = 0;
  function makeKey(): string {
    nextKey += 1;
    return `r${nextKey}`;
  }

  function rowFromTarget(t: DistributionTarget): Row {
    return {
      key: makeKey(),
      route_type: t.route_type,
      grade: t.grade,
      target_count: t.target_count,
    };
  }

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      listDistributionTargets(locId).catch(() => [] as DistributionTarget[]),
      listRoutes(locId, { status: 'active', limit: 500 }).catch(() => ({
        routes: [] as RouteShape[],
        total: 0,
        limit: 0,
        offset: 0,
      })),
    ])
      .then(([ts, rs]) => {
        if (cancelled) return;
        rows = ts.map(rowFromTarget);

        // Build observed-grades from active routes. Boulders contribute
        // V-grades, graded routes contribute their YDS grade, and circuit
        // routes contribute their circuit_color name (route.circuit_color).
        const seen = new Set<string>();
        const obs: { route_type: DistributionTargetType; grade: string }[] = [];
        for (const r of rs.routes) {
          let bucket: DistributionTargetType;
          let label: string;
          if (r.grading_system === 'circuit') {
            if (!r.circuit_color) continue;
            bucket = 'circuit';
            label = r.circuit_color;
          } else if (r.route_type === 'boulder') {
            bucket = 'boulder';
            label = r.grade;
          } else {
            bucket = 'route';
            label = r.grade;
          }
          const key = bucket + ':' + label;
          if (seen.has(key)) continue;
          seen.add(key);
          obs.push({ route_type: bucket, grade: label });
        }
        observedGrades = obs;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load targets.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  function addRow(route_type: DistributionTargetType = 'boulder', grade = '', target_count = 0) {
    rows = [...rows, { key: makeKey(), route_type, grade, target_count }];
  }

  function removeRow(key: string) {
    rows = rows.filter((r) => r.key !== key);
  }

  // Suggestions = observed grades not already covered by the current
  // editor state. One-click adds an empty target row pre-filled with
  // the bucket + grade.
  const suggestions = $derived.by(() => {
    const have = new Set(rows.map((r) => r.route_type + ':' + r.grade));
    return observedGrades
      .filter((g) => !have.has(g.route_type + ':' + g.grade))
      .sort((a, b) => {
        if (a.route_type !== b.route_type) return a.route_type.localeCompare(b.route_type);
        return a.grade.localeCompare(b.grade);
      });
  });

  async function save() {
    if (!locId || saving) return;
    // Surface obvious validation before round-tripping.
    for (const row of rows) {
      if (!row.grade.trim()) {
        error = 'Every target needs a grade label.';
        return;
      }
      if (row.target_count < 0 || !Number.isFinite(row.target_count)) {
        error = `Target count for ${row.grade} must be a non-negative number.`;
        return;
      }
    }
    saving = true;
    error = null;
    savedFlash = null;
    try {
      const updated = await replaceDistributionTargets(
        locId,
        rows.map((r) => ({
          route_type: r.route_type,
          grade: r.grade.trim(),
          target_count: Math.floor(r.target_count),
        })),
      );
      // Re-seed from the canonical server response (drops zero-count
      // rows the server filtered out).
      rows = updated.map(rowFromTarget);
      savedFlash = `Saved ${updated.length} target${updated.length === 1 ? '' : 's'}.`;
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Distribution targets — Routewerk</title>
</svelte:head>

<div class="page">
  <p><a class="back" href="/settings">← Settings</a></p>
  <h1>Distribution targets</h1>
  <p class="lede">
    Set how many active routes you want at each grade or circuit. The
    setter dashboard overlays these as targets on the distribution
    charts so the gap between actual and goal is visible at a glance.
    Changes here only affect the chart overlay — they don't touch the
    routes themselves.
  </p>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if !canEdit}
    <p class="muted">Head setter or above required.</p>
  {:else if loading}
    <p class="muted">Loading…</p>
  {:else}
    {#if error}<p class="error">{error}</p>{/if}
    {#if savedFlash}<p class="ok">{savedFlash}</p>{/if}

    <section class="card">
      <h2>Targets ({rows.length})</h2>
      {#if rows.length === 0}
        <p class="muted">
          No targets configured yet. Add rows below — or pick from the
          suggested grades that match your current route set.
        </p>
      {:else}
        <ul class="row-list">
          {#each rows as row (row.key)}
            <li class="row">
              <select bind:value={row.route_type}>
                <option value="boulder">Boulder</option>
                <option value="route">Route</option>
                <option value="circuit">Circuit</option>
              </select>
              <input
                class="grade-input"
                bind:value={row.grade}
                placeholder={row.route_type === 'boulder'
                  ? 'V4'
                  : row.route_type === 'route'
                  ? '5.10a'
                  : 'red'}
                maxlength="20" />
              <input
                class="count-input"
                type="number"
                min="0"
                bind:value={row.target_count}
                placeholder="0" />
              <button type="button" class="ghost danger" onclick={() => removeRow(row.key)}>
                Remove
              </button>
            </li>
          {/each}
        </ul>
      {/if}
      <div class="add-row">
        <button type="button" onclick={() => addRow()}>+ Add target</button>
      </div>
    </section>

    {#if suggestions.length > 0}
      <section class="card">
        <h2>Suggestions ({suggestions.length})</h2>
        <p class="muted small">
          Grades observed in your current active route set that don't
          have a target yet. Click to seed an empty target row.
        </p>
        <ul class="suggestion-list">
          {#each suggestions as s (s.route_type + ':' + s.grade)}
            <li>
              <button
                type="button"
                class="suggestion-chip"
                onclick={() => addRow(s.route_type, s.grade, 0)}>
                <span class="muted small">{s.route_type}</span>
                <span class="grade">{s.grade}</span>
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    <div class="actions">
      <button class="primary" disabled={saving} onclick={save}>
        {saving ? 'Saving…' : 'Save targets'}
      </button>
    </div>
  {/if}
</div>

<style>
  .page {
    max-width: 50rem;
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
    font-size: 0.85rem;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .card h2 {
    margin: 0 0 0.75rem;
    font-size: 1rem;
    font-weight: 600;
  }
  .row-list {
    list-style: none;
    padding: 0;
    margin: 0 0 0.75rem;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .row {
    display: grid;
    grid-template-columns: 8rem 1fr 6rem auto;
    gap: 8px;
    align-items: center;
  }
  .row select,
  .row input {
    padding: 0.45rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.9rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .grade-input {
    text-transform: capitalize;
  }
  .count-input {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .add-row {
    margin-top: 0.5rem;
  }
  .add-row button,
  button.ghost {
    background: transparent;
    border: 1px solid transparent;
    color: var(--rw-text-muted);
    padding: 0.35rem 0.75rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
  }
  .add-row button:hover,
  button.ghost:hover {
    border-color: var(--rw-border-strong);
    color: var(--rw-text);
  }
  button.ghost.danger:hover {
    color: var(--rw-danger);
    border-color: #fecaca;
  }
  .suggestion-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .suggestion-chip {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    background: var(--rw-surface-alt);
    border: 1px solid var(--rw-border);
    border-radius: 6px;
    padding: 4px 10px;
    font-size: 0.85rem;
    cursor: pointer;
    color: var(--rw-text);
  }
  .suggestion-chip:hover {
    border-color: var(--rw-accent);
  }
  .suggestion-chip .grade {
    font-weight: 600;
    text-transform: capitalize;
  }
  .actions {
    display: flex;
    gap: 8px;
    margin-top: 0.5rem;
  }
  button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border: 1px solid var(--rw-accent);
    padding: 0.55rem 1.1rem;
    border-radius: 8px;
    font-weight: 600;
    font-size: 0.9rem;
    cursor: pointer;
  }
  button.primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.85rem;
    border-radius: 6px;
    margin: 0 0 0.75rem;
    font-size: 0.9rem;
  }
  .ok {
    background: rgba(22, 163, 74, 0.1);
    border: 1px solid rgba(22, 163, 74, 0.3);
    color: #15803d;
    padding: 0.55rem 0.85rem;
    border-radius: 6px;
    margin: 0 0 0.75rem;
    font-size: 0.9rem;
  }
</style>
