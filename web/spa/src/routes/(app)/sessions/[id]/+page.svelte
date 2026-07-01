<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import {
    getSession,
    updateSession,
    deleteSession,
    updateSessionStatus,
    listWalls,
    listTeam,
    getLocationSettings,
    addSessionAssignment,
    removeSessionAssignment,
    listStripTargets,
    addStripTarget,
    removeStripTarget,
    listSessionChecklist,
    toggleChecklistItem,
    listRoutes,
    listSessionRoutes,
    addSessionRoute,
    updateSessionRoute,
    deleteSessionRoute,
    publishSession,
    ApiClientError,
    type SessionShape,
    type SessionStatus,
    type SessionWriteShape,
    type WallShape,
    type TeamMemberShape,
    type StripTargetShape,
    type ChecklistItemShape,
    type RouteShape,
    type SessionRouteDetailShape,
    type SessionRouteWriteShape,
    type LocationSettingsShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';
  import SessionForm from '$lib/components/SessionForm.svelte';
  import SessionClimbForm from '$lib/components/SessionClimbForm.svelte';

  let session = $state<SessionShape | null>(null);
  let walls = $state<WallShape[]>([]);
  let team = $state<TeamMemberShape[]>([]);
  let settings = $state<LocationSettingsShape | null>(null);
  let stripTargets = $state<StripTargetShape[]>([]);
  let checklist = $state<ChecklistItemShape[]>([]);
  let activeRoutes = $state<RouteShape[]>([]);
  let sessionRoutes = $state<SessionRouteDetailShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let editing = $state(false);
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let mutating = $state(false);
  let mutatingId = $state<string | null>(null);
  let mutateError = $state<string | null>(null);

  const sessionId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());
  const canManage = $derived(roleRankAt(locId) >= 3);
  const isComplete = $derived(session?.status === 'complete');
  // Building (add walls/climbs) is allowed while the session is still open.
  const canBuild = $derived(canManage && !isComplete);

  $effect(() => {
    if (!locId || !sessionId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getSession(locId, sessionId),
      listWalls(locId).catch(() => [] as WallShape[]),
      listTeam(locId).catch(() => ({ members: [], total_count: 0 })),
      // Gym settings drive the climb form's grade / circuit / color
      // pickers. Best-effort: setter+ can read, falls back to defaults.
      getLocationSettings(locId).catch(() => null),
      listStripTargets(locId, sessionId).catch(() => [] as StripTargetShape[]),
      listSessionChecklist(locId, sessionId).catch(() => [] as ChecklistItemShape[]),
      // Active routes feed the strip-target form's per-route picker.
      listRoutes(locId, { status: 'active', limit: 200 }).catch(() => ({
        routes: [],
        total: 0,
        limit: 0,
        offset: 0,
      })),
      // Routes built in THIS session (draft + already-published).
      listSessionRoutes(locId, sessionId).catch(() => [] as SessionRouteDetailShape[]),
    ])
      .then(([s, wls, tm, st8, st, cl, rt, sr]) => {
        if (cancelled) return;
        session = s;
        // Defensively default arrays — older API builds returned `null`
        // for empty result sets, which crashes any `.length` access.
        walls = wls ?? [];
        team = tm.members ?? [];
        settings = st8;
        stripTargets = st ?? [];
        checklist = cl ?? [];
        activeRoutes = rt.routes ?? [];
        sessionRoutes = sr ?? [];
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load session.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function refresh() {
    if (!locId || !sessionId) return;
    const [s, st, cl, sr, ar] = await Promise.all([
      getSession(locId, sessionId),
      listStripTargets(locId, sessionId).catch(() => stripTargets),
      listSessionChecklist(locId, sessionId).catch(() => checklist),
      listSessionRoutes(locId, sessionId).catch(() => sessionRoutes),
      // Keep the per-section "currently up" strip pills honest: publish
      // archives strip targets and activates drafts, so the active set
      // changes mid-session (publish → reopen especially).
      listRoutes(locId, { status: 'active', limit: 200 })
        .then((res) => res.routes ?? [])
        .catch(() => activeRoutes),
    ]);
    session = s;
    stripTargets = st;
    checklist = cl;
    sessionRoutes = sr;
    activeRoutes = ar;
  }

  async function refreshRoutes() {
    if (!locId || !sessionId) return;
    sessionRoutes = await listSessionRoutes(locId, sessionId);
  }

  async function submitEdit(body: SessionWriteShape) {
    if (!locId || !sessionId) return;
    saving = true;
    saveError = null;
    try {
      session = await updateSession(locId, sessionId, body);
      editing = false;
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      saving = false;
    }
  }

  async function changeStatus(newStatus: SessionStatus) {
    if (!locId || !sessionId || !session) return;
    mutating = true;
    mutateError = null;
    try {
      await updateSessionStatus(locId, sessionId, newStatus);
      session = { ...session, status: newStatus };
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Status change failed.';
    } finally {
      mutating = false;
    }
  }

  async function handleDelete() {
    if (!locId || !sessionId || !session) return;
    if (!confirm('Delete this session? Soft-delete; routes + assignments stay intact.')) return;
    mutating = true;
    try {
      await deleteSession(locId, sessionId);
      goto('/sessions');
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Delete failed.';
      mutating = false;
    }
  }

  // Publish summary surfaced after a successful one-shot.
  let publishResult = $state<{ stripped: number; published: number } | null>(null);
  let publishing = $state(false);
  let publishError = $state<string | null>(null);

  async function handlePublish() {
    if (!locId || !sessionId || !session) return;
    const draftCount = sessionRoutes.filter((r) => r.status === 'draft').length;
    const stripCount = stripTargets.length;
    if (draftCount === 0 && !confirm('No draft climbs to publish yet. Publish and complete this session anyway?')) {
      return;
    }
    const summary =
      `Publish this session?\n\nThis will:\n• activate ${draftCount} draft climb${draftCount === 1 ? '' : 's'}` +
      (stripCount > 0
        ? `\n• archive ${stripCount} strip-target${stripCount === 1 ? '' : 's'} (whole walls + individual routes)`
        : '') +
      `\n• flip the session to complete\n\nThis can't be undone in one click.`;
    if (!confirm(summary)) return;

    publishing = true;
    publishError = null;
    try {
      const res = await publishSession(locId, sessionId);
      publishResult = {
        stripped: res.stripped_route_count,
        published: res.published_routes,
      };
      await refresh();
    } catch (err) {
      publishError = err instanceof ApiClientError ? err.message : 'Publish failed.';
    } finally {
      publishing = false;
    }
  }

  // ── Setters that can be assigned / credited ─────────────────
  const settableSetters = $derived(
    team.filter(
      (m) =>
        m.role === 'setter' ||
        m.role === 'head_setter' ||
        m.role === 'gym_manager' ||
        m.role === 'org_admin',
    ),
  );

  // ── Wall sections (assignment + climbs grouped by wall) ─────
  const wallsById = $derived(new Map(walls.map((w) => [w.id, w])));

  // A wall is "in the session" if it has a wall-assignment or any climbs.
  const sectionWallIds = $derived.by(() => {
    const ids = new Set<string>();
    for (const a of session?.assignments ?? []) if (a.wall_id) ids.add(a.wall_id);
    for (const r of sessionRoutes) ids.add(r.wall_id);
    return ids;
  });

  // Ordered sections for walls we still know about (walls arrive sorted by
  // sort_order). Each section carries its setters + climbs.
  const wallSections = $derived(
    walls
      .filter((w) => sectionWallIds.has(w.id))
      .map((w) => ({
        wall: w,
        setters: (session?.assignments ?? []).filter((a) => a.wall_id === w.id),
        climbs: sessionRoutes.filter((r) => r.wall_id === w.id),
      })),
  );

  // Climbs whose wall is no longer listed (e.g. archived mid-session).
  const orphanClimbs = $derived(sessionRoutes.filter((r) => !wallsById.has(r.wall_id)));

  // Assignments not shown inside any wall section: either no wall, or a
  // wall that's no longer listed (archived mid-session). Surfacing the
  // latter keeps its Remove button reachable.
  const offWallAssignments = $derived(
    (session?.assignments ?? []).filter((a) => !a.wall_id || !wallsById.has(a.wall_id)),
  );

  const draftCount = $derived(sessionRoutes.filter((r) => r.status === 'draft').length);

  // ── Add wall sections (= assign setters to one or more walls) ──
  let sectionForm = $state({
    wall_ids: [] as string[],
    setter_ids: [] as string[],
    target_grades: [] as string[],
    notes: '',
  });
  let sectionSaving = $state(false);
  let sectionError = $state<string | null>(null);

  // Default grade lists — keep in sync with SessionClimbForm / RouteForm.
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

  // Target-grade chips follow the picked walls' types: boulder walls
  // offer V grades, rope walls YDS. Nothing picked (= whole-session
  // assignment) offers both.
  const targetGradeOptions = $derived.by((): string[] => {
    const picked = sectionForm.wall_ids
      .map((id) => wallsById.get(id))
      .filter((w): w is WallShape => !!w);
    const showV = picked.length === 0 || picked.some((w) => w.wall_type === 'boulder');
    const showYds = picked.length === 0 || picked.some((w) => w.wall_type === 'route');
    const v = settings?.grading.v_scale_range?.length
      ? settings.grading.v_scale_range
      : DEFAULT_V_GRADES;
    const yds = settings?.grading.yds_range?.length
      ? settings.grading.yds_range
      : DEFAULT_YDS_GRADES;
    return [...(showV ? v : []), ...(showYds ? yds : [])];
  });

  function toggleTargetGrade(g: string) {
    sectionForm.target_grades = sectionForm.target_grades.includes(g)
      ? sectionForm.target_grades.filter((x) => x !== g)
      : [...sectionForm.target_grades, g];
  }

  async function addWallSection(e: Event) {
    e.preventDefault();
    if (!locId || !sessionId) return;
    if (sectionForm.setter_ids.length === 0) {
      sectionError = 'Pick at least one setter.';
      return;
    }
    sectionSaving = true;
    sectionError = null;
    try {
      const notes = sectionForm.notes.trim() || null;
      // Keep only grades still offered (a wall-type change can strand a
      // pick from the other scale's chip set).
      const targetGrades = sectionForm.target_grades.filter((g) =>
        targetGradeOptions.includes(g),
      );
      // No walls picked → one whole-session (wall-less) assignment per
      // setter. Walls picked → one assignment per (wall, setter) pair. The
      // repo upserts on (session, setter, wall) — but ONLY for non-null
      // walls: Postgres treats NULLs as distinct in the unique constraint,
      // so re-adding a wall-less assignment would silently duplicate it.
      // Replace instead: remove each setter's existing wall-less row first,
      // preserving the update-grades/notes-in-place semantics.
      if (sectionForm.wall_ids.length === 0) {
        const existing = (session?.assignments ?? []).filter(
          (a) => !a.wall_id && sectionForm.setter_ids.includes(a.setter_id),
        );
        await Promise.all(
          existing.map((a) => removeSessionAssignment(locId, sessionId, a.id)),
        );
      }
      const wallTargets: (string | null)[] = sectionForm.wall_ids.length
        ? sectionForm.wall_ids
        : [null];
      await Promise.all(
        wallTargets.flatMap((wid) =>
          sectionForm.setter_ids.map((sid) =>
            addSessionAssignment(locId, sessionId, {
              setter_id: sid,
              wall_id: wid,
              target_grades: targetGrades.length ? targetGrades : undefined,
              notes,
            }),
          ),
        ),
      );
      sectionForm = { wall_ids: [], setter_ids: [], target_grades: [], notes: '' };
      await refresh();
    } catch (err) {
      sectionError = err instanceof ApiClientError ? err.message : 'Could not add to session.';
      await refresh(); // reflect any assignments that did land
    } finally {
      sectionSaving = false;
    }
  }

  // ── Per-section strip picking ─────────────────────────────
  // Current active routes on a section's wall, tap-to-strip. A route
  // already on the strip list shows as targeted; tapping again removes it.
  const stripTargetByRouteId = $derived.by(() => {
    const m = new Map<string, StripTargetShape>();
    for (const t of stripTargets) if (t.route_id) m.set(t.route_id, t);
    return m;
  });

  // Walls covered by a WHOLE-WALL strip target (route_id null). Their
  // routes are already coming down — pills render targeted + disabled so
  // a tap can't add a redundant per-route target.
  const wholeWallStripWalls = $derived.by(() => {
    const s = new Set<string>();
    for (const t of stripTargets) if (!t.route_id) s.add(t.wall_id);
    return s;
  });

  function activeRoutesOnWall(wallId: string): RouteShape[] {
    return activeRoutes.filter((r) => r.wall_id === wallId);
  }

  // Per-route in-flight tracking: a shared single id gets clobbered by
  // concurrent taps, briefly re-enabling an in-flight pill.
  let stripBusy = $state<Record<string, boolean>>({});

  async function toggleStripRoute(r: RouteShape) {
    if (!locId || !sessionId || stripBusy[r.id]) return;
    const existing = stripTargetByRouteId.get(r.id);
    stripBusy[r.id] = true;
    try {
      if (existing) {
        await removeStripTarget(locId, sessionId, existing.id);
      } else {
        await addStripTarget(locId, sessionId, { wall_id: r.wall_id, route_id: r.id });
      }
      stripTargets = await listStripTargets(locId, sessionId);
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not update strip list.';
    } finally {
      delete stripBusy[r.id];
    }
  }

  async function unassign(assignmentId: string) {
    if (!locId || !sessionId) return;
    mutatingId = assignmentId;
    try {
      await removeSessionAssignment(locId, sessionId, assignmentId);
      await refresh();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not remove setter.';
    } finally {
      mutatingId = null;
    }
  }

  function setterName(id: string): string {
    return team.find((m) => m.user_id === id)?.display_name ?? `Setter ${id.slice(0, 8)}…`;
  }

  // ── Climbs (draft routes built in this session) ─────────────
  let climbSaving = $state(false);
  let climbError = $state<string | null>(null);
  // Per-wall remount counters: bumping one clears only that wall's
  // add-climb form for the next entry, so a half-typed climb in another
  // section isn't wiped when you add a climb elsewhere.
  let climbFormNonces = $state<Record<string, number>>({});
  let editingClimbId = $state<string | null>(null);

  async function addClimb(body: SessionRouteWriteShape) {
    if (!locId || !sessionId) return;
    climbSaving = true;
    climbError = null;
    try {
      await addSessionRoute(locId, sessionId, body);
    } catch (err) {
      climbError = err instanceof ApiClientError ? err.message : 'Could not add climb.';
      climbSaving = false;
      return;
    }
    // The climb persisted. Clear THIS wall's form and re-sync routes; a
    // transient refresh failure must not look like a failed add (or strand
    // the just-entered values inviting a duplicate submit).
    climbFormNonces[body.wall_id] = (climbFormNonces[body.wall_id] ?? 0) + 1;
    try {
      await refreshRoutes();
    } catch {
      /* leave sessionRoutes as-is; next interaction re-syncs */
    } finally {
      climbSaving = false;
    }
  }

  async function saveClimbEdit(routeId: string, body: SessionRouteWriteShape) {
    if (!locId || !sessionId) return;
    climbSaving = true;
    climbError = null;
    try {
      await updateSessionRoute(locId, sessionId, routeId, body);
    } catch (err) {
      climbError = err instanceof ApiClientError ? err.message : 'Could not update climb.';
      climbSaving = false;
      return;
    }
    editingClimbId = null;
    try {
      await refreshRoutes();
    } catch {
      /* re-syncs on next interaction */
    } finally {
      climbSaving = false;
    }
  }

  async function removeClimb(routeId: string) {
    if (!locId || !sessionId) return;
    if (!confirm('Delete this draft climb?')) return;
    mutatingId = routeId;
    try {
      await deleteSessionRoute(locId, sessionId, routeId);
      await refreshRoutes();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not delete climb.';
    } finally {
      mutatingId = null;
    }
  }

  function climbGradeLabel(r: SessionRouteDetailShape): string {
    if (r.circuit_color) {
      const cc = r.circuit_color.charAt(0).toUpperCase() + r.circuit_color.slice(1);
      return r.grade && r.grade !== r.circuit_color ? `${cc} · ${r.grade}` : cc;
    }
    return r.grade;
  }

  // ── Strip targets ────────────────────────────────────────
  let stripForm = $state({ wall_id: '', route_id: '' });
  let stripSaving = $state(false);
  let stripError = $state<string | null>(null);

  const stripCandidateRoutes = $derived(
    stripForm.wall_id ? activeRoutes.filter((r) => r.wall_id === stripForm.wall_id) : [],
  );

  async function addStrip(e: Event) {
    e.preventDefault();
    if (!locId || !sessionId) return;
    if (!stripForm.wall_id) {
      stripError = 'Pick a wall.';
      return;
    }
    stripSaving = true;
    stripError = null;
    try {
      await addStripTarget(locId, sessionId, {
        wall_id: stripForm.wall_id,
        route_id: stripForm.route_id || null,
      });
      stripForm = { wall_id: '', route_id: '' };
      await refresh();
    } catch (err) {
      stripError = err instanceof ApiClientError ? err.message : 'Could not add strip target.';
    } finally {
      stripSaving = false;
    }
  }

  async function removeStrip(targetId: string) {
    if (!locId || !sessionId) return;
    mutatingId = targetId;
    try {
      await removeStripTarget(locId, sessionId, targetId);
      await refresh();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not remove target.';
    } finally {
      mutatingId = null;
    }
  }

  // ── Checklist ────────────────────────────────────────────
  async function toggleItem(itemId: string) {
    if (!locId || !sessionId) return;
    mutatingId = itemId;
    try {
      await toggleChecklistItem(locId, sessionId, itemId);
      checklist = await listSessionChecklist(locId, sessionId);
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not toggle item.';
    } finally {
      mutatingId = null;
    }
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleDateString(undefined, {
      weekday: 'long',
      month: 'long',
      day: 'numeric',
      year: 'numeric',
    });
  }

  function wallName(id: string | null | undefined): string {
    if (!id) return 'Any wall';
    return walls.find((w) => w.id === id)?.name ?? '—';
  }

  const STATUS_LABEL: Record<SessionStatus, string> = {
    planning: 'Planning',
    in_progress: 'In progress',
    complete: 'Complete',
    cancelled: 'Cancelled',
  };

  const allowedTransitions = $derived.by((): SessionStatus[] => {
    if (!session) return [];
    switch (session.status as SessionStatus) {
      case 'planning':
        return ['in_progress', 'cancelled'];
      case 'in_progress':
        return ['planning', 'cancelled'];
      case 'complete':
        return ['in_progress'];
      case 'cancelled':
        return ['planning'];
      default:
        return [];
    }
  });
</script>

<svelte:head>
  <title>Session {session ? fmtDate(session.scheduled_date) : ''} — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/sessions">← Sessions</a>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if session}
    <header class="page-header">
      <div>
        <h1>{fmtDate(session.scheduled_date)}</h1>
        <p class="meta-line">
          <span class="status-pill status-{session.status}">
            {STATUS_LABEL[session.status as SessionStatus] ?? session.status}
          </span>
          <span class="muted small">· {draftCount} draft climb{draftCount === 1 ? '' : 's'}</span>
        </p>
      </div>
      {#if !editing && canManage}
        <div class="header-actions">
          <a class="ghost-link" href="/sessions/{session.id}/photos">Photos →</a>
          <button onclick={() => (editing = true)}>Edit</button>
        </div>
      {/if}
    </header>

    {#if editing}
      <SessionForm
        initial={session}
        submitLabel="Save changes"
        onSubmit={submitEdit}
        onCancel={() => {
          editing = false;
          saveError = null;
        }}
        {saving}
        error={saveError} />
    {:else}
      {#if mutateError}<p class="error">{mutateError}</p>{/if}

      {#if session.notes}
        <section class="card">
          <h2>Notes</h2>
          <p class="prose">{session.notes}</p>
        </section>
      {/if}

      <!-- In-app help: the build workflow, shown while the session is open. -->
      {#if canBuild}
        <section class="card help-card">
          <h2>How to build this session</h2>
          <ol class="steps">
            <li><strong>Pick the walls you're setting</strong> and the setters on them (one or several of each) — each wall becomes a section below.</li>
            <li><strong>Add the climbs</strong> under each wall: grade/circuit, hold color, and (optionally) a name + who set it.</li>
            <li><strong>Schedule any strips</strong> — tap routes in each section's "currently up" list (or use the strip-targets card for whole walls) — and tick off the playbook checklist.</li>
            <li><strong>Review &amp; publish</strong> to activate every draft climb (and run the strips) in one shot.</li>
          </ol>
        </section>
      {/if}

      <!-- ── Wall sections ──────────────────────────────────── -->
      <section class="card">
        <div class="section-head">
          <h2>Wall sections</h2>
          <span class="count">{wallSections.length}</span>
        </div>

        {#if wallSections.length === 0 && orphanClimbs.length === 0}
          <p class="muted">
            No walls added yet.
            {#if canBuild}Start by picking the walls you're setting below.{/if}
          </p>
        {/if}

        {#each wallSections as sec (sec.wall.id)}
          <div class="wall-section">
            <div class="wall-section-head">
              <div>
                <span class="wall-name">{sec.wall.name}</span>
                <span class="wall-type">{sec.wall.wall_type}</span>
              </div>
              <span class="count small">{sec.climbs.length} climb{sec.climbs.length === 1 ? '' : 's'}</span>
            </div>

            <!-- Setters on this wall -->
            {#if sec.setters.length > 0}
              <ul class="setter-row">
                {#each sec.setters as a (a.id)}
                  <li class="setter-chip">
                    <span class="setter">{setterName(a.setter_id)}</span>
                    {#if a.target_grades && a.target_grades.length > 0}
                      <span class="muted">· {a.target_grades.join(', ')}</span>
                    {/if}
                    {#if a.notes}<span class="muted">· {a.notes}</span>{/if}
                    {#if canManage}
                      <button
                        class="chip-x"
                        title="Remove setter"
                        disabled={mutatingId === a.id}
                        onclick={() => unassign(a.id)}>×</button>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}

            <!-- Climbs on this wall -->
            {#if sec.climbs.length > 0}
              <ul class="climb-list">
                {#each sec.climbs as r (r.id)}
                  <li class="climb">
                    {#if editingClimbId === r.id}
                      <div class="climb-edit">
                        <SessionClimbForm
                          wall={sec.wall}
                          {settings}
                          setters={settableSetters}
                          initial={r}
                          submitLabel="Save"
                          saving={climbSaving}
                          error={climbError}
                          onSubmit={(body) => saveClimbEdit(r.id, body)}
                          onCancel={() => {
                            editingClimbId = null;
                            climbError = null;
                          }} />
                      </div>
                    {:else}
                      <span class="climb-color" style="background:{r.color || '#cbd5e1'}"></span>
                      <span class="climb-grade">{climbGradeLabel(r)}</span>
                      <span class="climb-info">
                        <span class="climb-name">{r.name ?? r.route_type}</span>
                        <span class="muted small">{r.setter_name}</span>
                      </span>
                      <span class="route-status {r.status}">{r.status}</span>
                      {#if canBuild && r.status === 'draft'}
                        <span class="climb-actions">
                          <button class="ghost small" onclick={() => (editingClimbId = r.id)}>Edit</button>
                          <button
                            class="ghost small"
                            disabled={mutatingId === r.id}
                            onclick={() => removeClimb(r.id)}>
                            {mutatingId === r.id ? '…' : 'Delete'}
                          </button>
                        </span>
                      {/if}
                    {/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="muted small no-climbs">No climbs on this wall yet.</p>
            {/if}

            <!-- Current routes on this wall — tap to add/remove from the
                 strip list without leaving the section. -->
            {#if canBuild && activeRoutesOnWall(sec.wall.id).length > 0}
              {@const wholeWall = wholeWallStripWalls.has(sec.wall.id)}
              <div class="section-strip">
                <span class="pick-label">
                  Currently up — tap to strip
                  {#if wholeWall}<span class="muted"> · whole wall already on the strip list</span>{/if}
                </span>
                <div class="strip-pills">
                  {#each activeRoutesOnWall(sec.wall.id) as r (r.id)}
                    {@const targeted = wholeWall || stripTargetByRouteId.has(r.id)}
                    <button type="button" class="strip-pill"
                            class:targeted
                            aria-pressed={targeted}
                            disabled={wholeWall || stripBusy[r.id]}
                            title={wholeWall
                              ? 'Covered by the whole-wall strip target'
                              : targeted
                                ? 'Remove from strip list'
                                : 'Add to strip list'}
                            onclick={() => toggleStripRoute(r)}>
                      <span class="strip-pill-color" style="background:{r.color || '#cbd5e1'}"></span>
                      <span>{r.grade}</span>
                      {#if r.name}<span class="muted">{r.name}</span>{/if}
                      {#if targeted}<span class="strip-mark">✕ strip</span>{/if}
                    </button>
                  {/each}
                </div>
              </div>
            {/if}

            <!-- Add a climb to this wall -->
            {#if canBuild}
              <div class="add-climb">
                {#key climbFormNonces[sec.wall.id] ?? 0}
                  <SessionClimbForm
                    wall={sec.wall}
                    {settings}
                    setters={settableSetters}
                    defaultSetterId={sec.setters[0]?.setter_id ?? ''}
                    submitLabel="Add climb"
                    saving={climbSaving}
                    onSubmit={addClimb} />
                {/key}
              </div>
            {/if}
          </div>
        {/each}

        <!-- Climbs whose wall is no longer listed (archived). Read-only. -->
        {#if orphanClimbs.length > 0}
          <div class="wall-section">
            <div class="wall-section-head">
              <span class="wall-name">Other climbs</span>
            </div>
            <ul class="climb-list">
              {#each orphanClimbs as r (r.id)}
                <li class="climb">
                  <span class="climb-color" style="background:{r.color || '#cbd5e1'}"></span>
                  <span class="climb-grade">{climbGradeLabel(r)}</span>
                  <span class="climb-info">
                    <span class="climb-name">{r.name ?? r.route_type}</span>
                    <span class="muted small">{r.wall_name} · {r.setter_name}</span>
                  </span>
                  <span class="route-status {r.status}">{r.status}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if climbError}<p class="error">{climbError}</p>{/if}

        <!-- Add wall section form -->
        {#if canBuild}
          {#if walls.length === 0}
            <p class="muted add-section-note">
              No walls at this gym yet — <a class="link" href="/walls/new">create a wall first</a>.
            </p>
          {:else}
            <form class="add-section" onsubmit={addWallSection}>
              <h3>Add wall sections</h3>
              <span class="pick-label">Setters — pick one or several *</span>
              <div class="wall-pick">
                {#each settableSetters as m (m.user_id)}
                  <label class="wall-check">
                    <input type="checkbox" bind:group={sectionForm.setter_ids} value={m.user_id} />
                    <span class="wall-check-name">{m.display_name}</span>
                  </label>
                {/each}
              </div>
              <span class="pick-label">Walls — pick one or several</span>
              <div class="wall-pick">
                {#each walls as w (w.id)}
                  <label class="wall-check" class:added={sectionWallIds.has(w.id)}>
                    <input type="checkbox" bind:group={sectionForm.wall_ids} value={w.id} />
                    <span class="wall-check-name">{w.name}</span>
                    <span class="wall-type">{w.wall_type}</span>
                  </label>
                {/each}
              </div>
              <p class="muted small hint">
                Leave walls unchecked to assign the setters to the whole session.
                Target grades &amp; notes apply to every picked wall.
              </p>
              <span class="pick-label">Target grades — tap to pick</span>
              <div class="chips target-chips">
                {#each targetGradeOptions as g (g)}
                  <button type="button" class="chip small-chip"
                          class:on={sectionForm.target_grades.includes(g)}
                          aria-pressed={sectionForm.target_grades.includes(g)}
                          onclick={() => toggleTargetGrade(g)}>{g}</button>
                {/each}
              </div>
              <div class="row">
                <label>
                  <span>Notes</span>
                  <input bind:value={sectionForm.notes} placeholder="optional" />
                </label>
              </div>
              {#if sectionError}<p class="error">{sectionError}</p>{/if}
              <button class="primary" type="submit" disabled={sectionSaving}>
                {sectionSaving ? 'Adding…' : 'Add to session'}
              </button>
            </form>
          {/if}
        {/if}
      </section>

      <!-- Setters not tied to a listed wall (whole-session, or a wall that
           was archived after assignment). -->
      {#if offWallAssignments.length > 0}
        <section class="card">
          <h2>Other assigned setters</h2>
          <p class="muted small">
            Assigned to the whole session, or to a wall that's since been archived.
          </p>
          <ul class="assignment-list">
            {#each offWallAssignments as a (a.id)}
              <li>
                <div class="assn-row">
                  <span>
                    <span class="setter">{setterName(a.setter_id)}</span>
                    {#if a.wall_id}<span class="muted small">· wall archived</span>{/if}
                    {#if a.target_grades && a.target_grades.length > 0}
                      <span class="muted small">· {a.target_grades.join(', ')}</span>
                    {/if}
                  </span>
                  {#if canManage}
                    <button
                      class="ghost small"
                      disabled={mutatingId === a.id}
                      onclick={() => unassign(a.id)}>
                      {mutatingId === a.id ? '…' : 'Remove'}
                    </button>
                  {/if}
                </div>
                {#if a.notes}<p class="assn-notes">{a.notes}</p>{/if}
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      <!-- ── Status / publish ──────────────────────────────── -->
      {#if canManage && (allowedTransitions.length > 0 || session.status !== 'complete')}
        <section class="card">
          <h2>Status</h2>
          <p class="muted small">
            Currently <strong>{STATUS_LABEL[session.status as SessionStatus]}</strong>.
            {#if session.status !== 'complete'}
              Publish activates every draft climb, archives every strip-target,
              and flips the session to complete in one shot.
            {/if}
          </p>
          {#if publishResult}
            <p class="ok">
              Published. {publishResult.published} draft{publishResult.published === 1 ? '' : 's'} activated;
              {publishResult.stripped} route{publishResult.stripped === 1 ? '' : 's'} archived.
            </p>
          {/if}
          {#if publishError}<p class="error">{publishError}</p>{/if}
          <div class="status-actions">
            {#if session.status !== 'complete'}
              <button class="primary" disabled={publishing || mutating} onclick={handlePublish}>
                {publishing ? 'Publishing…' : 'Review & publish'}
              </button>
            {/if}
            {#each allowedTransitions as next}
              <button disabled={mutating || publishing} onclick={() => changeStatus(next)}>
                Move to {STATUS_LABEL[next]}
              </button>
            {/each}
          </div>
        </section>
      {/if}

      <!-- ── Strip targets ─────────────────────────────────── -->
      {#if canManage}
        <section class="card">
          <h2>Strip targets</h2>
          <p class="muted small">
            Walls or specific routes to archive on completion. Whole-wall
            targets archive every active route on that wall.
          </p>
          {#if stripTargets.length === 0}
            <p class="muted">Nothing scheduled to strip.</p>
          {:else}
            <ul class="strip-list">
              {#each stripTargets as t (t.id)}
                <li>
                  <span class="strip-label">
                    {#if t.route_id}
                      <span class="color-chip" style="background:{t.route_color ?? '#cbd5e1'}"></span>
                      <strong>{t.route_grade}</strong>
                      {#if t.route_name}{t.route_name}{/if}
                      <span class="muted">on {t.wall_name}</span>
                    {:else}
                      <strong>Whole wall:</strong> {t.wall_name}
                    {/if}
                  </span>
                  <button
                    class="ghost small"
                    disabled={mutatingId === t.id}
                    onclick={() => removeStrip(t.id)}>
                    {mutatingId === t.id ? '…' : 'Remove'}
                  </button>
                </li>
              {/each}
            </ul>
          {/if}

          {#if canBuild}
            <form class="strip-form" onsubmit={addStrip}>
              <h3>Add target</h3>
              <div class="row">
                <label>
                  <span>Wall *</span>
                  <select bind:value={stripForm.wall_id}>
                    <option value="">Pick a wall…</option>
                    {#each walls as w (w.id)}
                      <option value={w.id}>{w.name}</option>
                    {/each}
                  </select>
                </label>
                <label>
                  <span>Route (or leave blank for whole wall)</span>
                  <select bind:value={stripForm.route_id} disabled={!stripForm.wall_id}>
                    <option value="">Whole wall</option>
                    {#each stripCandidateRoutes as r (r.id)}
                      <option value={r.id}>
                        {r.grade}
                        {#if r.name}— {r.name}{/if}
                      </option>
                    {/each}
                  </select>
                </label>
              </div>
              {#if stripError}<p class="error">{stripError}</p>{/if}
              <button class="primary" type="submit" disabled={stripSaving}>
                {stripSaving ? 'Adding…' : 'Add target'}
              </button>
            </form>
          {/if}
        </section>
      {/if}

      <!-- ── Checklist ─────────────────────────────────────── -->
      {#if checklist.length > 0}
        <section class="card">
          <h2>Checklist</h2>
          <ul class="checklist">
            {#each checklist as item (item.id)}
              <li class="check-item" class:done={item.completed}>
                <button
                  class="check-box"
                  class:done={item.completed}
                  disabled={mutatingId === item.id}
                  onclick={() => toggleItem(item.id)}
                  aria-label={item.completed ? 'Mark not done' : 'Mark done'}>
                  {item.completed ? '✓' : ''}
                </button>
                <span class="check-title">{item.title}</span>
                {#if item.completed && item.completed_by_name}
                  <span class="muted small">by {item.completed_by_name}</span>
                {/if}
              </li>
            {/each}
          </ul>
        </section>
      {/if}

      {#if canManage}
        <section class="card danger-zone">
          <h2>Danger zone</h2>
          <p class="muted">Soft-delete this session. Routes + assignments stay intact.</p>
          <button class="danger" disabled={mutating} onclick={handleDelete}>
            {mutating ? '…' : 'Delete session'}
          </button>
        </section>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
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
  .page-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: 1rem;
    margin-bottom: 1.5rem;
  }
  .header-actions {
    display: inline-flex;
    gap: 8px;
    align-items: center;
  }
  .ghost-link {
    color: var(--rw-text);
    text-decoration: none;
    border: 1px solid var(--rw-border-strong);
    padding: 0.4rem 0.85rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 600;
  }
  .ghost-link:hover {
    border-color: var(--rw-accent);
    color: var(--rw-accent);
  }
  h1 {
    font-size: 1.5rem;
    font-weight: 700;
    margin: 0;
    letter-spacing: -0.01em;
  }
  h3 {
    font-size: 0.9rem;
    font-weight: 600;
    margin: 0.85rem 0 0.5rem;
  }
  .meta-line {
    margin: 6px 0 0;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .status-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .status-planning {
    background: rgba(59, 130, 246, 0.12);
    color: #1d4ed8;
  }
  .status-in_progress {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .status-complete {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .status-cancelled {
    background: var(--rw-surface-alt);
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
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.75rem;
  }
  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }
  .section-head h2 {
    margin: 0;
  }
  .count {
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
    border-radius: 999px;
    padding: 1px 9px;
    font-size: 0.8rem;
    font-weight: 700;
  }
  .small {
    font-size: 0.85rem;
  }
  .prose {
    margin: 0;
    color: var(--rw-text);
    line-height: 1.5;
    white-space: pre-wrap;
  }

  /* In-app help */
  .help-card {
    background: var(--rw-surface-alt, #f8fafc);
    border-style: dashed;
  }
  .steps {
    margin: 0;
    padding-left: 1.2rem;
    display: flex;
    flex-direction: column;
    gap: 6px;
    color: var(--rw-text);
    font-size: 0.9rem;
    line-height: 1.45;
  }

  /* Wall sections */
  .wall-section {
    border: 1px solid var(--rw-border);
    border-radius: 10px;
    padding: 0.85rem 0.95rem;
    margin-bottom: 0.85rem;
  }
  .wall-section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 0.5rem;
  }
  .wall-name {
    font-weight: 700;
    font-size: 1rem;
  }
  .wall-type {
    margin-left: 8px;
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--rw-text-muted);
    background: var(--rw-surface-alt);
    padding: 1px 7px;
    border-radius: 4px;
  }
  .setter-row {
    list-style: none;
    margin: 0 0 0.6rem;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .setter-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    background: var(--rw-surface-alt);
    border-radius: 999px;
    padding: 2px 6px 2px 10px;
    font-size: 0.82rem;
  }
  .chip-x {
    border: none;
    background: transparent;
    color: var(--rw-text-muted);
    font-size: 1rem;
    line-height: 1;
    padding: 0 2px;
    cursor: pointer;
    font-weight: 700;
  }
  .chip-x:hover:not(:disabled) {
    color: #b91c1c;
  }
  .climb-list {
    list-style: none;
    margin: 0 0 0.6rem;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .climb {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0.4rem 0.5rem;
    border-radius: 7px;
    background: var(--rw-surface-alt, #f8fafc);
  }
  .climb-edit {
    flex: 1;
    padding: 0.35rem 0;
  }
  .climb-color {
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
    flex-shrink: 0;
  }
  .climb-grade {
    font-weight: 700;
    font-size: 0.9rem;
    min-width: 3.5rem;
  }
  .climb-info {
    flex: 1;
    display: flex;
    flex-direction: column;
    line-height: 1.2;
  }
  .climb-name {
    font-size: 0.9rem;
    font-weight: 600;
  }
  .climb-actions {
    display: inline-flex;
    gap: 4px;
  }
  .route-status {
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-weight: 700;
    padding: 1px 7px;
    border-radius: 4px;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .route-status.draft {
    background: rgba(245, 158, 11, 0.18);
    color: #92590a;
  }
  .route-status.active {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
  }
  .no-climbs {
    margin: 0 0 0.6rem;
  }
  .add-climb {
    border-top: 1px dashed var(--rw-border);
    padding-top: 0.6rem;
  }
  .section-strip {
    border-top: 1px dashed var(--rw-border);
    padding: 0.6rem 0;
    margin-bottom: 0.6rem;
  }
  .strip-pills {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .strip-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: 1px solid var(--rw-border-strong);
    border-radius: 999px;
    padding: 3px 10px;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.8rem;
    font-weight: 600;
  }
  .strip-pill.targeted {
    border-color: #fca5a5;
    background: #fef2f2;
    color: #991b1b;
  }
  .strip-pill-color {
    width: 11px;
    height: 11px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
    flex-shrink: 0;
  }
  .strip-mark {
    font-size: 0.68rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .target-chips {
    margin-bottom: 0.6rem;
  }
  .chip {
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    padding: 0.3rem 0.6rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.8rem;
    font-weight: 600;
    min-width: 2.3rem;
  }
  .chip.on {
    border-color: var(--rw-accent);
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
  }
  .add-section,
  .strip-form {
    margin-top: 1rem;
    padding-top: 0.85rem;
    border-top: 1px dashed var(--rw-border);
  }
  .add-section-note {
    margin-top: 0.85rem;
  }
  .pick-label {
    display: block;
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--rw-text-muted);
    margin-bottom: 0.35rem;
  }
  .wall-pick {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 0.35rem;
  }
  .wall-check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    border: 1px solid var(--rw-border-strong);
    border-radius: 999px;
    padding: 4px 11px 4px 9px;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--rw-text);
    cursor: pointer;
    flex-direction: row;
    user-select: none;
  }
  .wall-check input {
    margin: 0;
    accent-color: var(--rw-accent);
    cursor: pointer;
  }
  .wall-check.added {
    border-style: dashed;
  }
  .wall-check.added .wall-check-name::after {
    content: ' ✓';
    color: var(--rw-accent);
  }
  .wall-check:has(input:checked) {
    border-color: var(--rw-accent);
    background: var(--rw-surface-alt);
  }
  .wall-check .wall-type {
    margin-left: 0;
  }
  .hint {
    margin: 0 0 0.6rem;
  }

  .assignment-list,
  .strip-list,
  .checklist {
    list-style: none;
    padding: 0;
    margin: 0 0 0.5rem;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .assignment-list li {
    border-top: 1px solid var(--rw-border);
    padding-top: 8px;
  }
  .assignment-list li:first-child {
    border-top: none;
    padding-top: 0;
  }
  .assn-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }
  .setter {
    font-weight: 600;
  }
  .assn-notes {
    margin: 4px 0 0;
    font-size: 0.9rem;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
    margin-bottom: 0.6rem;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--rw-text-muted);
    font-weight: 600;
  }
  select,
  input {
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-family: inherit;
  }
  select:focus,
  input:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  .strip-list li {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .strip-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
  }
  .color-chip {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    border: 1px solid var(--rw-border-strong);
  }
  .status-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .check-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0.45rem 0;
    border-top: 1px solid var(--rw-border);
  }
  .check-item:first-child {
    border-top: none;
  }
  .check-item.done .check-title {
    color: var(--rw-text-muted);
    text-decoration: line-through;
  }
  .check-box {
    width: 22px;
    height: 22px;
    border-radius: 4px;
    border: 2px solid var(--rw-border-strong);
    background: var(--rw-surface);
    cursor: pointer;
    color: var(--rw-accent-ink);
    font-weight: 700;
    padding: 0;
    line-height: 1;
    flex-shrink: 0;
  }
  .check-box.done {
    background: var(--rw-accent);
    border-color: var(--rw-accent);
  }
  .check-title {
    flex: 1;
    color: var(--rw-text);
    font-size: 0.92rem;
  }
  .danger-zone {
    border-color: #fde2e2;
    background: #fffafa;
  }
  .danger-zone h2 {
    color: #991b1b;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
  }
  button {
    cursor: pointer;
    padding: 0.5rem 0.85rem;
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
  button.ghost {
    background: transparent;
  }
  button.small {
    font-size: 0.75rem;
    padding: 0.3rem 0.6rem;
  }
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
    background: #fff;
  }
  button.danger:hover:not(:disabled) {
    background: #fef2f2;
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
