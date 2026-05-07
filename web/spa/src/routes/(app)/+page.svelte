<script lang="ts">
  import {
    getDashboardSummary,
    listWalls,
    listRoutes,
    ApiClientError,
    type DashboardSummaryShape,
    type WallShape,
    type RouteShape,
  } from '$lib/api/client';
  import { currentUser, roleRankAt } from '$lib/stores/auth.svelte';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  const me = $derived(currentUser());
  const locId = $derived(effectiveLocationId());
  // Stats panel only renders for setter+ (server enforces with the same
  // RequireLocationRole gate). Climbers see the original quick-actions card.
  const isStaff = $derived(roleRankAt(locId) >= 2);

  let summary = $state<DashboardSummaryShape | null>(null);
  let summaryLoading = $state(false);
  let summaryError = $state<string | null>(null);

  // Wall-by-route grid (setter dashboard). Mirrors the HTMX
  // dashboard's per-wall list — derived client-side from listWalls +
  // listRoutes(active) so we don't need a dedicated endpoint.
  let walls = $state<WallShape[]>([]);
  let activeRoutes = $state<RouteShape[]>([]);

  $effect(() => {
    if (!locId || !isStaff) {
      summary = null;
      walls = [];
      activeRoutes = [];
      return;
    }
    let cancelled = false;
    summaryLoading = true;
    summaryError = null;
    Promise.all([
      getDashboardSummary(locId),
      listWalls(locId).catch(() => [] as WallShape[]),
      listRoutes(locId, { status: 'active', limit: 500 }).catch(() => ({
        routes: [],
        total: 0,
        limit: 0,
        offset: 0,
      })),
    ])
      .then(([s, wls, rt]) => {
        if (cancelled) return;
        summary = s;
        walls = wls;
        activeRoutes = rt.routes;
      })
      .catch((err) => {
        if (cancelled) return;
        summaryError = err instanceof ApiClientError ? err.message : 'Could not load dashboard.';
      })
      .finally(() => {
        if (!cancelled) summaryLoading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  // Group routes by wall for the per-wall grid. Walls with zero active
  // routes still render so setters spot empty walls at a glance.
  const wallsWithRoutes = $derived.by(() => {
    const byWall = new Map<string, RouteShape[]>();
    for (const r of activeRoutes) {
      if (!byWall.has(r.wall_id)) byWall.set(r.wall_id, []);
      byWall.get(r.wall_id)!.push(r);
    }
    // Sort routes within a wall by grade for stable display.
    for (const list of byWall.values()) {
      list.sort((a, b) => a.grade.localeCompare(b.grade));
    }
    return walls.map((w) => ({
      wall: w,
      routes: byWall.get(w.id) ?? [],
    }));
  });

  const QUICK_ACTIONS = [
    { label: 'Browse routes', href: '/routes', desc: 'See active routes by wall.' },
    { label: 'Manage walls', href: '/walls', desc: 'Edit wall layout + angles.' },
    {
      label: 'Setting sessions',
      href: '/sessions',
      desc: 'Plan + track route-setting work.',
    },
    {
      label: 'Competitions',
      href: '/competitions',
      desc: 'Build comps, manage events + registrations.',
    },
  ];

  function fmtAge(iso: string): string {
    const ms = Date.now() - new Date(iso).getTime();
    const sec = Math.floor(ms / 1000);
    if (sec < 60) return `${sec}s ago`;
    const min = Math.floor(sec / 60);
    if (min < 60) return `${min}m ago`;
    const hr = Math.floor(min / 60);
    if (hr < 24) return `${hr}h ago`;
    return new Date(iso).toLocaleDateString();
  }

  function displayGrade(a: { route_grade: string; route_grading_system: string; route_circuit_color?: string | null }): string {
    if (a.route_grading_system === 'circuit' && a.route_circuit_color) {
      return a.route_circuit_color;
    }
    return a.route_grade;
  }

  // ── Route distribution charts (setter+) ─────────────────────────
  // The user wants a quick "what's the gym's mix look like right now"
  // glance. Derived purely from activeRoutes already in scope, so no
  // extra API calls — bars are computed client-side.

  type DistBucket = {
    key: string;
    label: string;
    count: number;
    color?: string; // optional fill (circuit hex when grouping by circuit)
    section: 'boulder' | 'route' | 'circuit'; // sort grouping
  };

  /**
   * Order grades within a section so V0..V10 doesn't sort lexicographically
   * (V10 ahead of V2). Pulls the trailing number for V-scale and YDS, falls
   * back to localeCompare. Null grades sort last.
   */
  function gradeKey(grade: string): number {
    if (!grade) return 9999;
    const v = grade.match(/^V(\d+)/i);
    if (v) return parseInt(v[1], 10);
    const yds = grade.match(/^5\.(\d+)/);
    if (yds) {
      let base = parseInt(yds[1], 10) * 10;
      // Letter suffix gives a/b/c/d ordering within a grade.
      const letter = grade.match(/^5\.\d+([a-d])/i);
      if (letter) base += letter[1].toLowerCase().charCodeAt(0) - 96;
      const sign = grade.match(/^5\.\d+([+-])$/);
      if (sign) base += sign[1] === '+' ? 5 : -5;
      return base;
    }
    return 5000;
  }

  // Distribution grouped by grade. Boulders (V-scale) and routes
  // (YDS / letter / circuit) get separate sub-lists so the chart
  // doesn't show a 50% chunk of "circuit" alongside "5.10a"-style
  // grades.
  const gradeDistribution = $derived.by<DistBucket[]>(() => {
    const buckets = new Map<string, DistBucket>();
    for (const r of activeRoutes) {
      // Skip circuit-graded routes here — they're covered by the
      // circuit chart below. We only want grade-on-grade comparison.
      if (r.grading_system === 'circuit') continue;
      const section: DistBucket['section'] = r.route_type === 'boulder' ? 'boulder' : 'route';
      const key = section + ':' + r.grade;
      if (!buckets.has(key)) {
        buckets.set(key, { key, label: r.grade, count: 0, section });
      }
      buckets.get(key)!.count++;
    }
    return [...buckets.values()].sort((a, b) => {
      if (a.section !== b.section) return a.section === 'boulder' ? -1 : 1;
      return gradeKey(a.label) - gradeKey(b.label);
    });
  });

  // Distribution grouped by circuit color. Pulls the chip color from
  // the first route in each bucket so the bars show the actual circuit
  // hex even when the gym has custom palette presets. Routes without a
  // circuit_color (i.e. graded routes) are skipped.
  const circuitDistribution = $derived.by<DistBucket[]>(() => {
    const buckets = new Map<string, DistBucket>();
    for (const r of activeRoutes) {
      if (!r.circuit_color) continue;
      const key = 'circuit:' + r.circuit_color;
      if (!buckets.has(key)) {
        buckets.set(key, {
          key,
          label: r.circuit_color,
          count: 0,
          color: r.color,
          section: 'circuit',
        });
      }
      buckets.get(key)!.count++;
    }
    // Sort by count desc — most-stocked circuit first reads better
    // than alpha order for a "what's our mix" glance.
    return [...buckets.values()].sort((a, b) => b.count - a.count);
  });

  // Max count drives bar widths (one scale per chart so the longest
  // bar always pegs to 100%).
  const gradeMax = $derived(gradeDistribution.reduce((m, b) => Math.max(m, b.count), 0));
  const circuitMax = $derived(circuitDistribution.reduce((m, b) => Math.max(m, b.count), 0));
</script>

<svelte:head>
  <title>Dashboard — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <h1>
      Welcome back{#if me}, {me.display_name.split(' ')[0]}{/if}.
    </h1>
    <p class="lede">
      {isStaff ? 'Quick look at the gym' : 'Pick up where you left off, or jump to a workflow below.'}
    </p>
  </header>

  {#if isStaff && locId}
    {#if summaryLoading && !summary}
      <p class="muted">Loading…</p>
    {:else if summaryError}
      <p class="error">{summaryError}</p>
    {:else if summary}
      <section class="stat-grid">
        <div class="stat-card">
          <div class="stat-value">{summary.stats.active_routes}</div>
          <div class="stat-label">Active routes</div>
          {#if summary.stats.active_delta !== 0}
            <div class="stat-delta" class:up={summary.stats.active_delta > 0}>
              {summary.stats.active_delta > 0 ? '↑' : '↓'} {Math.abs(summary.stats.active_delta)} this week
            </div>
          {/if}
        </div>
        <div class="stat-card">
          <div class="stat-value">{summary.stats.total_sends_30d}</div>
          <div class="stat-label">Sends (30d)</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">
            {summary.stats.avg_rating > 0 ? summary.stats.avg_rating.toFixed(1) : '—'}
          </div>
          <div class="stat-label">Avg rating</div>
        </div>
        <div class="stat-card" class:warn={summary.stats.due_for_strip > 0}>
          <div class="stat-value">{summary.stats.due_for_strip}</div>
          <div class="stat-label">Due for strip</div>
        </div>
      </section>

      <section class="card">
        <h2>Recent activity</h2>
        {#if summary.recent_activity.length === 0}
          <p class="muted">No ascents in the last few days.</p>
        {:else}
          <ul class="activity-list">
            {#each summary.recent_activity as a}
              <li>
                <div class="avatar-fallback">{a.user_name?.[0]?.toUpperCase() ?? '?'}</div>
                <div class="activity-body">
                  <span class="user">{a.user_name}</span>
                  <span class="action type-{a.ascent_type}">{a.ascent_type}</span>
                  <span class="route">
                    <span class="color-chip" style="background:{a.route_color}"></span>
                    {displayGrade(a)}{#if a.route_name}<span class="rname">· {a.route_name}</span>{/if}
                  </span>
                </div>
                <span class="age muted">{fmtAge(a.time)}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      {#if gradeDistribution.length > 0 || circuitDistribution.length > 0}
        <section class="dist-grid">
          {#if gradeDistribution.length > 0}
            <div class="card">
              <h2>Active routes by grade</h2>
              <p class="muted small">
                Snapshot of the current set. Boulders sort V-scale ascending,
                then graded routes ascend by YDS.
              </p>
              <ul class="dist-bars">
                {#each gradeDistribution as b (b.key)}
                  <li>
                    <span class="dist-label">{b.label}</span>
                    <span class="dist-bar-track">
                      <span
                        class="dist-bar-fill grade-fill"
                        style="width: {(b.count / gradeMax) * 100}%"></span>
                    </span>
                    <span class="dist-count">{b.count}</span>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if circuitDistribution.length > 0}
            <div class="card">
              <h2>Active circuits</h2>
              <p class="muted small">
                Routes assigned to a circuit color. Sorted by count — biggest
                circuits first.
              </p>
              <ul class="dist-bars">
                {#each circuitDistribution as b (b.key)}
                  <li>
                    <span class="dist-label dist-label-circuit">
                      <span class="color-chip" style="background:{b.color}" aria-hidden="true"></span>
                      {b.label}
                    </span>
                    <span class="dist-bar-track">
                      <span
                        class="dist-bar-fill"
                        style="width: {(b.count / circuitMax) * 100}%; background: {b.color};"></span>
                    </span>
                    <span class="dist-count">{b.count}</span>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </section>
      {/if}

      {#if walls.length > 0}
        <section class="card">
          <h2>Walls &amp; routes</h2>
          <p class="muted small">
            Active routes per wall. Click a route to see ascents + ratings.
          </p>
          <ul class="wall-list">
            {#each wallsWithRoutes as { wall, routes } (wall.id)}
              <li>
                <div class="wall-row">
                  <a class="wall-name" href="/walls/{wall.id}">
                    {wall.name}
                    <span class="wall-type muted">{wall.wall_type}</span>
                  </a>
                  <span class="route-count muted">
                    {routes.length} active
                  </span>
                </div>
                {#if routes.length === 0}
                  <p class="muted small empty-wall">No active routes — set or unstrip something.</p>
                {:else}
                  <ul class="route-chips">
                    {#each routes as r (r.id)}
                      <li>
                        <a
                          class="route-chip"
                          href="/routes/{r.id}"
                          title="{r.grade}{r.name ? ' · ' + r.name : ''}">
                          <span class="color-chip" style="background:{r.color}"></span>
                          <span class="chip-grade">{r.grade}</span>
                        </a>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {/if}
    {/if}
  {/if}

  <section class="card-grid">
    {#each QUICK_ACTIONS as action (action.href)}
      <a class="action-card" href={action.href}>
        <span class="action-label">{action.label}</span>
        <span class="action-desc">{action.desc}</span>
        <span class="action-arrow" aria-hidden="true">→</span>
      </a>
    {/each}
  </section>

  {#if !locId}
    <p class="hint">
      You're not a member of any location yet. Ask your gym admin for an invite.
    </p>
  {/if}
</div>

<style>
  .page {
    max-width: 64rem;
  }
  .page-header {
    margin-bottom: 1.5rem;
  }
  h1 {
    font-size: 1.75rem;
    font-weight: 700;
    margin: 0 0 0.35rem;
    letter-spacing: -0.01em;
  }
  h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.85rem;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
    font-size: 1rem;
  }
  .stat-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 0.85rem;
    margin-bottom: 1rem;
  }
  .stat-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1rem 1.1rem;
  }
  .stat-card.warn {
    border-color: rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.06);
  }
  .stat-value {
    font-size: 1.6rem;
    font-weight: 700;
    line-height: 1;
    margin-bottom: 0.3rem;
  }
  .stat-label {
    color: var(--rw-text-muted);
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .stat-delta {
    font-size: 0.75rem;
    color: var(--rw-text-muted);
    margin-top: 0.4rem;
  }
  .stat-delta.up {
    color: #15803d;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.1rem 1.25rem;
    margin-bottom: 1rem;
  }
  .activity-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .activity-list li {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 10px;
    align-items: center;
    padding: 0.45rem 0;
    border-top: 1px solid var(--rw-border);
  }
  .activity-list li:first-child {
    border-top: none;
  }
  .avatar-fallback {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.75rem;
  }
  .activity-body {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    font-size: 0.9rem;
  }
  .user {
    font-weight: 600;
  }
  .action {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 1px 7px;
    border-radius: 4px;
    font-weight: 700;
  }
  .type-send,
  .type-flash {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .type-attempt {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .route {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--rw-text);
    font-weight: 600;
  }
  .color-chip {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .rname {
    font-weight: 500;
    color: var(--rw-text-muted);
  }
  .age {
    font-size: 0.8rem;
  }
  .small {
    font-size: 0.85rem;
  }
  .dist-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(20rem, 1fr));
    gap: 1rem;
    margin-bottom: 1rem;
  }
  .dist-grid .card {
    margin: 0;
  }
  .dist-bars {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .dist-bars > li {
    display: grid;
    grid-template-columns: 4.5rem 1fr 2.5rem;
    align-items: center;
    gap: 10px;
    font-size: 0.85rem;
  }
  .dist-label {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
    color: var(--rw-text);
    text-align: right;
  }
  .dist-label-circuit {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    text-align: left;
    text-transform: capitalize;
    justify-content: flex-end;
  }
  .dist-bar-track {
    background: var(--rw-surface-alt);
    border-radius: 4px;
    height: 14px;
    position: relative;
    overflow: hidden;
  }
  .dist-bar-fill {
    display: block;
    height: 100%;
    border-radius: 4px;
    background: var(--rw-accent);
    transition: width 200ms ease;
  }
  .dist-bar-fill.grade-fill {
    background: linear-gradient(90deg, var(--rw-accent) 0%, var(--rw-accent-hover) 100%);
  }
  .dist-count {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--rw-text-muted);
    font-size: 0.8rem;
  }
  .wall-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .wall-list > li {
    border-top: 1px solid var(--rw-border);
    padding-top: 10px;
  }
  .wall-list > li:first-child {
    border-top: none;
    padding-top: 4px;
  }
  .wall-row {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 6px;
  }
  .wall-name {
    color: var(--rw-text);
    text-decoration: none;
    font-weight: 600;
    font-size: 0.95rem;
  }
  .wall-name:hover {
    color: var(--rw-accent);
  }
  .wall-type {
    margin-left: 6px;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    font-weight: 600;
  }
  .route-count {
    font-size: 0.8rem;
  }
  .empty-wall {
    margin: 4px 0 0;
  }
  .route-chips {
    list-style: none;
    padding: 0;
    margin: 4px 0 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .route-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 8px;
    border: 1px solid var(--rw-border);
    border-radius: 999px;
    text-decoration: none;
    color: var(--rw-text);
    font-size: 0.8rem;
    font-weight: 600;
    transition: border-color 120ms;
  }
  .route-chip:hover {
    border-color: var(--rw-accent);
  }
  .chip-grade {
    line-height: 1;
  }
  .card-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
    gap: 1rem;
  }
  .action-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem 1.25rem 1rem;
    text-decoration: none;
    color: inherit;
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 4px 12px;
    align-items: start;
    transition: border-color 120ms, transform 120ms;
  }
  .action-card:hover {
    border-color: var(--rw-accent);
    transform: translateY(-1px);
  }
  .action-label {
    font-weight: 600;
    font-size: 1rem;
    color: var(--rw-text);
  }
  .action-desc {
    grid-column: 1 / 2;
    color: var(--rw-text-muted);
    font-size: 0.9rem;
  }
  .action-arrow {
    grid-column: 2 / 3;
    grid-row: 1 / 2;
    color: var(--rw-text-faint);
    font-size: 1.2rem;
    transition: transform 120ms, color 120ms;
  }
  .action-card:hover .action-arrow {
    color: var(--rw-accent);
    transform: translateX(2px);
  }
  .hint {
    margin-top: 2rem;
    color: var(--rw-text-faint);
    font-size: 0.92rem;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.85rem;
    border-radius: 8px;
  }
</style>
