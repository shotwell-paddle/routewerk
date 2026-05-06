<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    authState,
    isAuthenticated,
    currentUser,
    effectiveRoleAt,
    roleRankAt,
  } from '$lib/stores/auth.svelte';
  import {
    locationState,
    setSelectedLocation,
    effectiveLocationId,
  } from '$lib/stores/location.svelte';
  import { getLocation, type LocationShape } from '$lib/api/client';

  let { children } = $props();

  // Cache location metadata so the picker doesn't refetch on every render.
  let locationsById = $state<Record<string, LocationShape>>({});

  // Auth gate at the layout level — every /app/* page inherits it.
  $effect(() => {
    const a = authState();
    if (a.loaded && a.me === null) {
      goto('/sign-in?next=' + encodeURIComponent(page.url.pathname + page.url.search));
    }
  });

  onMount(async () => {
    // Wait for /me to settle before fetching location metadata.
    while (!authState().loaded) {
      await new Promise((r) => setTimeout(r, 30));
    }
    if (!isAuthenticated()) return;

    // Default to the first location membership if nothing stored.
    if (!locationState().selectedId) {
      const fallback = effectiveLocationId();
      if (fallback) setSelectedLocation(fallback);
    }

    // Load metadata for every location the user has access to so the
    // picker can show names instead of UUIDs.
    const me = authState().me!;
    const ids = Array.from(
      new Set(me.memberships.map((m) => m.location_id).filter((x): x is string => !!x)),
    );
    const results = await Promise.all(ids.map((id) => getLocation(id)));
    const next: Record<string, LocationShape> = {};
    results.forEach((loc, i) => {
      if (loc) next[ids[i]] = loc;
    });
    locationsById = next;
  });

  const me = $derived(currentUser());
  const selectedLocId = $derived(locationState().selectedId);
  const selectedLoc = $derived(selectedLocId ? locationsById[selectedLocId] : null);

  // Locations the user can pick — derived from memberships, deduped.
  const accessibleLocations = $derived.by(() => {
    const meV = authState().me;
    if (!meV) return [];
    const seen = new Set<string>();
    return meV.memberships
      .filter((m) => {
        if (!m.location_id || seen.has(m.location_id)) return false;
        seen.add(m.location_id);
        return true;
      })
      .map((m) => ({
        location_id: m.location_id!,
        role: m.role,
        location: locationsById[m.location_id!],
      }));
  });

  // Best role at the selected location, sourced from the shared helper.
  // Mirrors the server's bestRole + app-admin promotion. Without the
  // org-wide membership fallback, an org_admin (whose membership row has
  // location_id=null) would get a climber-flavored sidebar even though
  // the API treats them as admin.
  const selectedRole = $derived(effectiveRoleAt(selectedLocId));
  const roleRank = $derived(roleRankAt(selectedLocId));

  // Quests are gated by the location's progressions_enabled flag.
  // Only show the nav link when the selected location has it on.
  const progressionsEnabled = $derived<boolean>(
    selectedLoc ? !!selectedLoc.progressions_enabled : false,
  );

  type NavItem = {
    label: string;
    href: string;
    minRoleRank: number;
    group: 'main' | 'staff';
    visible?: () => boolean;
  };
  const NAV: NavItem[] = [
    // Climber + everyone
    { label: 'Dashboard', href: '/app', minRoleRank: 0, group: 'main' },
    { label: 'Walls', href: '/app/walls', minRoleRank: 0, group: 'main' },
    { label: 'Routes', href: '/app/routes', minRoleRank: 0, group: 'main' },
    {
      label: 'Quests',
      href: '/app/quests',
      minRoleRank: 1,
      group: 'main',
      visible: () => progressionsEnabled,
    },
    { label: 'Profile', href: '/app/profile', minRoleRank: 1, group: 'main' },
    // Staff
    { label: 'Sessions', href: '/app/sessions', minRoleRank: 2, group: 'staff' },
    { label: 'Card batches', href: '/app/card-batches', minRoleRank: 2, group: 'staff' },
    { label: 'Competitions', href: '/staff/comp', minRoleRank: 4, group: 'staff' },
    { label: 'Team', href: '/app/team', minRoleRank: 3, group: 'staff' },
    { label: 'Settings', href: '/app/settings', minRoleRank: 4, group: 'staff' },
  ];

  const visibleNav = $derived(
    NAV.filter((n) => roleRank >= n.minRoleRank && (n.visible ? n.visible() : true)),
  );

  function isActive(href: string): boolean {
    if (href === '/app') return page.url.pathname === '/app';
    return page.url.pathname.startsWith(href);
  }

  function onLocationChange(e: Event) {
    const v = (e.target as HTMLSelectElement).value;
    setSelectedLocation(v || null);
  }

  let mobileNavOpen = $state(false);
</script>

<div class="app-shell">
  <aside class="sidebar" class:open={mobileNavOpen}>
    <div class="brand">
      <a href="/app" class="brand-link">
        <span class="brand-mark">RW</span>
        <span class="brand-name">Routewerk</span>
      </a>
    </div>

    {#if accessibleLocations.length > 0}
      <div class="loc-picker">
        <label for="loc-select" class="loc-label">Location</label>
        <select id="loc-select" value={selectedLocId ?? ''} onchange={onLocationChange}>
          {#each accessibleLocations as opt (opt.location_id)}
            <option value={opt.location_id}>
              {opt.location?.name ?? '…'}
            </option>
          {/each}
        </select>
        {#if selectedRole}
          <span class="role-pill">{selectedRole.replace('_', ' ')}</span>
        {/if}
      </div>
    {/if}

    <nav class="nav">
      {#if visibleNav.some((n) => n.group === 'main')}
        <div class="nav-section">
          <span class="nav-section-label">Workspace</span>
          {#each visibleNav.filter((n) => n.group === 'main') as item (item.href)}
            <a class="nav-link" class:active={isActive(item.href)} href={item.href}>
              {item.label}
            </a>
          {/each}
        </div>
      {/if}

      {#if visibleNav.some((n) => n.group === 'staff')}
        <div class="nav-section">
          <span class="nav-section-label">Staff</span>
          {#each visibleNav.filter((n) => n.group === 'staff') as item (item.href)}
            <a class="nav-link" class:active={isActive(item.href)} href={item.href}>
              {item.label}
            </a>
          {/each}
        </div>
      {/if}
    </nav>

    {#if me}
      <div class="user-card">
        <div class="user-avatar">{me.display_name?.[0]?.toUpperCase() ?? '?'}</div>
        <div class="user-info">
          <div class="user-name">{me.display_name}</div>
          <div class="user-email">{me.email}</div>
        </div>
        <a class="signout-link" href="/logout" title="Sign out">Sign out</a>
      </div>
    {/if}
  </aside>

  <div class="content-region">
    <header class="topbar">
      <button
        class="hamburger"
        aria-label="Toggle navigation"
        onclick={() => (mobileNavOpen = !mobileNavOpen)}>
        <span></span><span></span><span></span>
      </button>
      <span class="topbar-title">
        {selectedLoc?.name ?? 'Routewerk'}
      </span>
    </header>
    <main class="main">
      {#if !authState().loaded}
        <p class="loading-shell">Loading…</p>
      {:else if isAuthenticated()}
        {@render children()}
      {/if}
    </main>
  </div>

  {#if mobileNavOpen}
    <button
      class="backdrop"
      aria-label="Close navigation"
      onclick={() => (mobileNavOpen = false)}></button>
  {/if}
</div>

<style>
  .app-shell {
    display: grid;
    grid-template-columns: 260px 1fr;
    min-height: 100vh;
  }

  /* ── Sidebar ──────────────────────────────────────────── */
  /* Mirrors the HTMX shell at web/static/css/routewerk.css so the
     SPA feels like one app with the HTMX surfaces, not a separate
     visual tier. Warm-black gradient, uppercase letter-spaced brand,
     orange accent. */
  .sidebar {
    --sidebar-text:        #ffffff;
    --sidebar-text-muted:  rgba(255, 255, 255, 0.55);
    --sidebar-text-faint:  rgba(255, 255, 255, 0.3);
    --sidebar-bg-elevated: rgba(255, 255, 255, 0.04);
    --sidebar-border:      rgba(255, 255, 255, 0.06);
    --sidebar-active-bg:   rgba(255, 255, 255, 0.07);

    background: linear-gradient(180deg, #161514 0%, #0e0d0c 100%);
    color: var(--sidebar-text);
    display: flex;
    flex-direction: column;
    padding: 0;
    border-right: 1px solid rgba(255, 255, 255, 0.04);
  }

  .brand {
    padding: 32px 24px 16px;
    border-bottom: 1px solid var(--sidebar-border);
  }
  .brand-link {
    display: inline-flex;
    align-items: center;
    gap: 0.55rem;
    text-decoration: none;
    color: var(--sidebar-text);
  }
  .brand-mark {
    width: 22px;
    height: 22px;
    border-radius: 4px;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0.65rem;
    font-weight: 800;
    letter-spacing: 0.5px;
  }
  .brand-name {
    font-size: 0.875rem;
    font-weight: 800;
    text-transform: uppercase;
    letter-spacing: 0.14em;
    line-height: 1;
  }

  /* Location picker */
  .loc-picker {
    background: var(--sidebar-bg-elevated);
    border: 1px solid rgba(255, 255, 255, 0.07);
    border-radius: 6px;
    padding: 0.5rem 0.65rem;
    margin: 12px 16px 14px;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }
  .loc-label {
    font-size: 0.65rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--sidebar-text-faint);
  }
  .loc-picker select {
    background: transparent;
    color: var(--sidebar-text);
    border: none;
    font-size: 0.85rem;
    padding: 0;
    appearance: none;
    cursor: pointer;
    font-weight: 500;
  }
  .loc-picker select:focus-visible {
    outline: none;
  }
  .loc-picker select option {
    color: #1c1b18;
  }
  .role-pill {
    display: inline-block;
    align-self: flex-start;
    background: rgba(252, 82, 0, 0.18);
    color: var(--rw-accent);
    padding: 1px 8px;
    border-radius: 4px;
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  /* Nav */
  .nav {
    flex: 1;
    overflow-y: auto;
    padding: 8px 12px;
  }
  .nav-section {
    margin-bottom: 16px;
  }
  .nav-section-label {
    display: block;
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--sidebar-text-faint);
    padding: 6px 12px 8px;
  }
  .nav-link {
    display: block;
    padding: 8px 12px;
    border-radius: 6px;
    color: var(--sidebar-text-muted);
    text-decoration: none;
    font-size: 0.875rem;
    font-weight: 500;
    position: relative;
    transition: background 120ms, color 120ms;
  }
  .nav-link:hover {
    background: var(--sidebar-bg-elevated);
    color: var(--sidebar-text);
  }
  .nav-link.active {
    background: var(--sidebar-active-bg);
    color: var(--sidebar-text);
  }
  .nav-link.active::before {
    content: '';
    position: absolute;
    left: -12px;
    top: 8px;
    bottom: 8px;
    width: 3px;
    border-radius: 0 2px 2px 0;
    background: var(--rw-accent);
  }

  /* User card at bottom — mirrors HTMX's .sidebar-footer / .user-pill. */
  .user-card {
    display: flex;
    align-items: center;
    gap: 10px;
    border-top: 1px solid var(--sidebar-border);
    padding: 14px 16px;
    margin-top: 0;
  }
  .user-avatar {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-weight: 700;
    font-size: 0.85rem;
    flex-shrink: 0;
  }
  .user-card .user-info {
    flex: 1;
    min-width: 0;
  }
  .user-name {
    color: var(--sidebar-text);
    font-weight: 600;
    font-size: 0.85rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .user-email {
    color: var(--sidebar-text-faint);
    font-size: 0.7rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .signout-link {
    color: var(--sidebar-text-muted);
    font-size: 0.75rem;
    text-decoration: none;
    padding: 4px 8px;
    border-radius: 4px;
    border: 1px solid rgba(255, 255, 255, 0.1);
  }
  .signout-link:hover {
    color: var(--sidebar-text);
    border-color: rgba(255, 255, 255, 0.2);
  }

  /* ── Content region ──────────────────────────────────── */
  .content-region {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .topbar {
    display: none;
  }
  .main {
    flex: 1;
    padding: 1.5rem 2rem;
  }
  .loading-shell {
    color: var(--rw-text-faint);
  }

  /* ── Mobile ──────────────────────────────────────────── */
  .hamburger {
    background: transparent;
    border: 1px solid var(--rw-border);
    border-radius: 6px;
    padding: 6px 8px;
    cursor: pointer;
    display: inline-flex;
    flex-direction: column;
    gap: 3px;
  }
  .hamburger span {
    display: block;
    width: 18px;
    height: 2px;
    background: var(--rw-text);
    border-radius: 1px;
  }
  .backdrop {
    display: none;
  }

  @media (max-width: 768px) {
    .app-shell {
      grid-template-columns: 1fr;
    }
    .sidebar {
      position: fixed;
      inset: 0 auto 0 0;
      width: 280px;
      transform: translateX(-100%);
      transition: transform 200ms ease;
      z-index: 30;
    }
    .sidebar.open {
      transform: translateX(0);
    }
    .topbar {
      display: flex;
      align-items: center;
      gap: 0.75rem;
      padding: 0.75rem 1rem;
      border-bottom: 1px solid var(--rw-border);
      background: var(--rw-surface);
      position: sticky;
      top: 0;
      z-index: 10;
    }
    .topbar-title {
      font-weight: 600;
    }
    .main {
      padding: 1rem;
    }
    .backdrop {
      display: block;
      position: fixed;
      inset: 0;
      background: rgba(15, 20, 34, 0.5);
      border: none;
      cursor: pointer;
      z-index: 20;
    }
  }
</style>
