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
    addSessionAssignment,
    removeSessionAssignment,
    listStripTargets,
    addStripTarget,
    removeStripTarget,
    listSessionChecklist,
    toggleChecklistItem,
    listRoutes,
    ApiClientError,
    type SessionShape,
    type SessionStatus,
    type SessionWriteShape,
    type WallShape,
    type TeamMemberShape,
    type StripTargetShape,
    type ChecklistItemShape,
    type RouteShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';
  import SessionForm from '$lib/components/SessionForm.svelte';

  let session = $state<SessionShape | null>(null);
  let walls = $state<WallShape[]>([]);
  let team = $state<TeamMemberShape[]>([]);
  let stripTargets = $state<StripTargetShape[]>([]);
  let checklist = $state<ChecklistItemShape[]>([]);
  let activeRoutes = $state<RouteShape[]>([]);
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

  $effect(() => {
    if (!locId || !sessionId) return;
    let cancelled = false;
    loading = true;
    error = null;
    Promise.all([
      getSession(locId, sessionId),
      listWalls(locId).catch(() => [] as WallShape[]),
      listTeam(locId).catch(() => ({ members: [], total_count: 0 })),
      listStripTargets(locId, sessionId).catch(() => [] as StripTargetShape[]),
      listSessionChecklist(locId, sessionId).catch(() => [] as ChecklistItemShape[]),
      // Active routes feed the strip-target form's per-route picker.
      listRoutes(locId, { status: 'active', limit: 200 }).catch(() => ({
        routes: [],
        total: 0,
        limit: 0,
        offset: 0,
      })),
    ])
      .then(([s, wls, tm, st, cl, rt]) => {
        if (cancelled) return;
        session = s;
        walls = wls;
        team = tm.members;
        stripTargets = st;
        checklist = cl;
        activeRoutes = rt.routes;
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
    const [s, st, cl] = await Promise.all([
      getSession(locId, sessionId),
      listStripTargets(locId, sessionId).catch(() => stripTargets),
      listSessionChecklist(locId, sessionId).catch(() => checklist),
    ]);
    session = s;
    stripTargets = st;
    checklist = cl;
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
      goto('/app/sessions');
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Delete failed.';
      mutating = false;
    }
  }

  // ── Assignments ──────────────────────────────────────────

  let assignForm = $state({
    setter_id: '',
    wall_id: '',
    notes: '',
  });
  let assignSaving = $state(false);
  let assignError = $state<string | null>(null);

  async function addAssignment(e: Event) {
    e.preventDefault();
    if (!locId || !sessionId) return;
    if (!assignForm.setter_id) {
      assignError = 'Pick a setter.';
      return;
    }
    assignSaving = true;
    assignError = null;
    try {
      await addSessionAssignment(locId, sessionId, {
        setter_id: assignForm.setter_id,
        wall_id: assignForm.wall_id || null,
        notes: assignForm.notes.trim() || null,
      });
      assignForm = { setter_id: '', wall_id: '', notes: '' };
      await refresh();
    } catch (err) {
      assignError = err instanceof ApiClientError ? err.message : 'Could not add assignment.';
    } finally {
      assignSaving = false;
    }
  }

  async function unassign(assignmentId: string) {
    if (!locId || !sessionId) return;
    mutatingId = assignmentId;
    try {
      await removeSessionAssignment(locId, sessionId, assignmentId);
      await refresh();
    } catch (err) {
      mutateError = err instanceof ApiClientError ? err.message : 'Could not remove assignment.';
    } finally {
      mutatingId = null;
    }
  }

  function setterName(id: string): string {
    return team.find((m) => m.user_id === id)?.display_name ?? `Setter ${id.slice(0, 8)}…`;
  }

  // ── Strip targets ────────────────────────────────────────

  let stripForm = $state({
    wall_id: '',
    route_id: '', // empty means "whole wall"
  });
  let stripSaving = $state(false);
  let stripError = $state<string | null>(null);

  // Active routes filtered to the wall the user picks. The strip-target
  // form lets you target a specific route OR the whole wall.
  const stripCandidateRoutes = $derived(
    stripForm.wall_id
      ? activeRoutes.filter((r) => r.wall_id === stripForm.wall_id)
      : [],
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

  // Status transitions allowed from the current state.
  const allowedTransitions = $derived.by((): SessionStatus[] => {
    if (!session) return [];
    switch (session.status as SessionStatus) {
      case 'planning':
        return ['in_progress', 'cancelled'];
      case 'in_progress':
        return ['planning', 'cancelled']; // 'complete' goes through HTMX
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
  <a class="back" href="/app/sessions">← Sessions</a>

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
        </p>
      </div>
      {#if !editing && canManage}
        <button onclick={() => (editing = true)}>Edit</button>
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

      {#if canManage && allowedTransitions.length > 0}
        <section class="card">
          <h2>Status</h2>
          <p class="muted small">
            Currently <strong>{STATUS_LABEL[session.status as SessionStatus]}</strong>.
            {#if session.status !== 'complete'}
              For final publish (which strips routes + publishes drafts),
              use <a class="link" href="/sessions/{session.id}/complete">the existing complete view</a>.
            {/if}
          </p>
          <div class="status-actions">
            {#each allowedTransitions as next}
              <button disabled={mutating} onclick={() => changeStatus(next)}>
                Move to {STATUS_LABEL[next]}
              </button>
            {/each}
          </div>
        </section>
      {/if}

      <section class="card">
        <h2>Assignments</h2>
        {#if !session.assignments || session.assignments.length === 0}
          <p class="muted">No setters assigned yet.</p>
        {:else}
          <ul class="assignment-list">
            {#each session.assignments as a (a.id)}
              <li>
                <div class="assn-row">
                  <div>
                    <span class="setter">{setterName(a.setter_id)}</span>
                    <span class="muted">
                      · {wallName(a.wall_id)}
                      {#if a.target_grades && a.target_grades.length > 0}
                        · grades: {a.target_grades.join(', ')}
                      {/if}
                    </span>
                  </div>
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
        {/if}

        {#if canManage}
          <form class="assn-form" onsubmit={addAssignment}>
            <h3>Add a setter</h3>
            <div class="row">
              <label>
                <span>Setter</span>
                <select bind:value={assignForm.setter_id}>
                  <option value="">Pick a setter…</option>
                  {#each team.filter((m) => m.role === 'setter' || m.role === 'head_setter' || m.role === 'gym_manager' || m.role === 'org_admin') as m (m.user_id)}
                    <option value={m.user_id}>{m.display_name}</option>
                  {/each}
                </select>
              </label>
              <label>
                <span>Wall (optional)</span>
                <select bind:value={assignForm.wall_id}>
                  <option value="">Any wall</option>
                  {#each walls as w (w.id)}
                    <option value={w.id}>{w.name}</option>
                  {/each}
                </select>
              </label>
            </div>
            <label>
              <span>Notes</span>
              <textarea bind:value={assignForm.notes} rows="2" placeholder="Anything specific for this setter…"></textarea>
            </label>
            {#if assignError}<p class="error">{assignError}</p>{/if}
            <button class="primary" type="submit" disabled={assignSaving}>
              {assignSaving ? 'Adding…' : 'Add assignment'}
            </button>
          </form>
        {/if}
      </section>

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
        </section>
      {/if}

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
  .small {
    font-size: 0.85rem;
  }
  .prose {
    margin: 0;
    color: var(--rw-text);
    line-height: 1.5;
    white-space: pre-wrap;
  }
  .status-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
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
  .assignment-list li,
  .strip-list li {
    border-top: 1px solid var(--rw-border);
    padding-top: 8px;
  }
  .assignment-list li:first-child,
  .strip-list li:first-child {
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
  .assn-form,
  .strip-form {
    margin-top: 1rem;
    padding-top: 0.85rem;
    border-top: 1px dashed var(--rw-border);
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
  textarea {
    padding: 0.5rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-family: inherit;
  }
  select:focus,
  textarea:focus {
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
</style>
