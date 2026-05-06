<script lang="ts">
  import {
    listTeam,
    updateMembership,
    removeMembership,
    ApiClientError,
    type TeamMemberShape,
    type MembershipRole,
  } from '$lib/api/client';
  import { authState } from '$lib/stores/auth.svelte';
  import { effectiveLocationId, locationState } from '$lib/stores/location.svelte';

  let members = $state<TeamMemberShape[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let q = $state('');
  let roleFilter = $state<'' | MembershipRole>('');
  let mutatingId = $state<string | null>(null);

  const locId = $derived(effectiveLocationId());

  // Caller's role at the selected location — drives which actions are
  // visible. The server is the source of truth; this just hides
  // affordances the caller can't use.
  const callerRole = $derived.by((): MembershipRole | null => {
    const me = authState().me;
    if (!me) return null;
    const sel = locationState().selectedId;
    if (!sel) return null;
    return (
      (me.memberships.find((m) => m.location_id === sel)?.role as MembershipRole) ?? null
    );
  });
  const ROLE_RANK: Record<MembershipRole, number> = {
    climber: 1,
    setter: 2,
    head_setter: 3,
    gym_manager: 4,
    org_admin: 5,
  };
  const callerRank = $derived(callerRole ? ROLE_RANK[callerRole] : 0);

  // The role options a caller of `callerRank` is allowed to assign.
  // Mirrors handler/team.go::allowedRolesForGrantor.
  const ASSIGNABLE_ROLES = $derived.by((): MembershipRole[] => {
    if (callerRank >= 5) return ['climber', 'setter', 'head_setter', 'gym_manager', 'org_admin'];
    if (callerRank >= 4) return ['climber', 'setter', 'head_setter', 'gym_manager'];
    if (callerRank >= 3) return ['climber', 'setter'];
    return [];
  });
  const canRemove = $derived(callerRank >= 4);

  $effect(() => {
    if (!locId) return;
    let cancelled = false;
    loading = true;
    error = null;
    listTeam(locId, { q: q.trim() || undefined, role: roleFilter || undefined })
      .then((res) => {
        if (!cancelled) members = res.members;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load team.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function changeRole(m: TeamMemberShape, newRole: MembershipRole) {
    if (newRole === m.role) return;
    mutatingId = m.membership_id;
    try {
      await updateMembership(m.membership_id, newRole);
      m.role = newRole;
      members = [...members];
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Role update failed.';
    } finally {
      mutatingId = null;
    }
  }

  async function remove(m: TeamMemberShape) {
    if (!confirm(`Remove ${m.display_name} from the team? They can re-join later.`)) return;
    mutatingId = m.membership_id;
    try {
      await removeMembership(m.membership_id);
      members = members.filter((x) => x.membership_id !== m.membership_id);
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Remove failed.';
    } finally {
      mutatingId = null;
    }
  }

  const ROLE_LABEL: Record<MembershipRole, string> = {
    climber: 'Climber',
    setter: 'Setter',
    head_setter: 'Head setter',
    gym_manager: 'Gym manager',
    org_admin: 'Org admin',
  };
</script>

<svelte:head>
  <title>Team — Routewerk</title>
</svelte:head>

<div class="page">
  <header class="page-header">
    <div>
      <h1>Team</h1>
      <p class="lede">Manage who can set, manage, and admin at this location.</p>
    </div>
  </header>

  {#if !locId}
    <p class="muted">Pick a location from the sidebar.</p>
  {:else if callerRank < 3}
    <p class="muted">
      Only head setters and above can manage the team. Switch locations or ask a
      head setter for access.
    </p>
  {:else}
    <div class="filter-bar">
      <label>
        <span>Search</span>
        <input type="search" bind:value={q} placeholder="Name or email…" />
      </label>
      <label>
        <span>Role</span>
        <select bind:value={roleFilter}>
          <option value="">Any</option>
          <option value="climber">Climber</option>
          <option value="setter">Setter</option>
          <option value="head_setter">Head setter</option>
          <option value="gym_manager">Gym manager</option>
          <option value="org_admin">Org admin</option>
        </select>
      </label>
    </div>

    {#if error}<p class="error">{error}</p>{/if}

    {#if loading && members.length === 0}
      <p class="muted">Loading team…</p>
    {:else if members.length === 0}
      <div class="empty-card">
        <h3>No team members match</h3>
        <p>Try clearing the filters.</p>
      </div>
    {:else}
      <ul class="member-list">
        {#each members as m (m.membership_id)}
          {@const targetRank = ROLE_RANK[m.role] ?? 0}
          {@const canActOnTarget = callerRank > targetRank || callerRank >= 5}
          {@const isSelf = authState().me?.user.id === m.user_id}
          <li class="member" class:is-self={isSelf}>
            <div class="who">
              <div class="avatar-fallback">{m.display_name?.[0]?.toUpperCase() ?? '?'}</div>
              <div>
                <div class="name">
                  {m.display_name}
                  {#if isSelf}<span class="self-tag">you</span>{/if}
                </div>
                <div class="email muted">{m.email}</div>
              </div>
            </div>
            <div class="role-cell">
              {#if isSelf}
                <!-- Self-role display: never editable on this page. The
                     server also blocks self-demotion. To change your own
                     role, ask another manager or org admin. -->
                <span class="role-pill role-{m.role}">{ROLE_LABEL[m.role]}</span>
              {:else if canActOnTarget && ASSIGNABLE_ROLES.length > 0}
                <select
                  value={m.role}
                  disabled={mutatingId === m.membership_id}
                  onchange={(e) => changeRole(m, (e.currentTarget.value as MembershipRole))}>
                  {#each ASSIGNABLE_ROLES as r}
                    <option value={r}>{ROLE_LABEL[r]}</option>
                  {/each}
                  {#if !ASSIGNABLE_ROLES.includes(m.role)}
                    <option value={m.role} disabled>{ROLE_LABEL[m.role]}</option>
                  {/if}
                </select>
              {:else}
                <span class="role-pill role-{m.role}">{ROLE_LABEL[m.role]}</span>
              {/if}
            </div>
            <div class="actions">
              {#if canRemove && canActOnTarget}
                <button
                  class="danger"
                  disabled={mutatingId === m.membership_id}
                  onclick={() => remove(m)}>
                  {mutatingId === m.membership_id ? '…' : 'Remove'}
                </button>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 56rem;
  }
  .page-header {
    margin-bottom: 1.5rem;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.25rem;
    letter-spacing: -0.01em;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
  }
  .filter-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 0.85rem;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 0.85rem 1rem;
    margin-bottom: 1rem;
  }
  .filter-bar label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 0.8rem;
    color: var(--rw-text-muted);
    font-weight: 600;
    flex: 1 1 12rem;
  }
  .filter-bar input,
  .filter-bar select {
    padding: 0.45rem 0.65rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.92rem;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  .member-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .member {
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 12px;
    align-items: center;
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 10px;
    padding: 0.7rem 1rem;
  }
  .who {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }
  .avatar-fallback {
    width: 36px;
    height: 36px;
    border-radius: 50%;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
  }
  .name {
    font-weight: 600;
    color: var(--rw-text);
  }
  .email {
    font-size: 0.85rem;
  }
  .role-cell select {
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.85rem;
    font-weight: 600;
  }
  .role-pill {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
  }
  .role-org_admin,
  .role-gym_manager {
    background: rgba(124, 58, 237, 0.12);
    color: #6d28d9;
  }
  .role-head_setter {
    background: rgba(59, 130, 246, 0.12);
    color: #1d4ed8;
  }
  .role-setter {
    background: rgba(22, 163, 74, 0.12);
    color: #15803d;
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
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
  }
  button.danger:hover:not(:disabled) {
    background: #fef2f2;
  }
  .empty-card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 2rem 1.5rem;
    text-align: center;
  }
  .empty-card h3 {
    margin: 0 0 0.4rem;
  }
  .empty-card p {
    color: var(--rw-text-muted);
    margin: 0;
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
    margin-bottom: 0.75rem;
  }
</style>
