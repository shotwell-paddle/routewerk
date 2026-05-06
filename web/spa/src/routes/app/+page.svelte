<script lang="ts">
  import { currentUser } from '$lib/stores/auth.svelte';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  const me = $derived(currentUser());
  const locId = $derived(effectiveLocationId());

  // Quick actions surface what most users come here to do. Keep this list
  // tight — it's the landing page, not a feature inventory.
  const QUICK_ACTIONS = [
    { label: 'Browse routes', href: '/app/routes', desc: 'See active routes by wall.' },
    { label: 'Manage walls', href: '/app/walls', desc: 'Edit wall layout + angles.' },
    {
      label: 'Setting sessions',
      href: '/app/sessions',
      desc: 'Plan + track route-setting work.',
    },
    {
      label: 'Competitions',
      href: '/staff/comp',
      desc: 'Build comps, manage events + registrations.',
    },
  ];
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
      Pick up where you left off, or jump to a workflow below.
    </p>
  </header>

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
    margin-bottom: 2rem;
  }
  h1 {
    font-size: 1.75rem;
    font-weight: 700;
    margin: 0 0 0.35rem;
    letter-spacing: -0.01em;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0;
    font-size: 1rem;
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
</style>
