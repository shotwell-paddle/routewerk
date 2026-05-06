<script lang="ts">
  import type { CompetitionProblem, AttemptState } from '$lib/api/client';

  interface Props {
    problem: CompetitionProblem;
    state: AttemptState | undefined;
    href: string;
  }

  let { problem, state, href }: Props = $props();

  // Status pill — shows the climber the at-a-glance result for this
  // problem. Top wins over zone, zone over attempts, attempts over
  // untouched.
  const status = $derived.by(() => {
    if (!state || (state.attempts === 0 && !state.zone_reached && !state.top_reached)) {
      return { label: 'untouched', kind: 'idle' as const };
    }
    if (state.top_reached) {
      const tries = state.attempts === 1 ? 'flash' : `${state.attempts} tries`;
      return { label: `top · ${tries}`, kind: 'top' as const };
    }
    if (state.zone_reached) {
      return { label: `zone · ${state.attempts} tries`, kind: 'zone' as const };
    }
    return { label: `${state.attempts} tries`, kind: 'tries' as const };
  });
</script>

<a {href} class="card" data-status={status.kind}>
  <div class="left">
    <div class="label">{problem.label}</div>
    {#if problem.color || problem.grade}
      <div class="meta">
        {#if problem.color}<span class="color-dot" style:background={problem.color}></span>{/if}
        {#if problem.grade}<span>{problem.grade}</span>{/if}
      </div>
    {/if}
  </div>
  <div class="right">
    <span class="status">{status.label}</span>
  </div>
</a>

<style>
  .card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 1rem;
    background: #fff;
    border: 1px solid #e2e8f0;
    border-radius: 10px;
    text-decoration: none;
    color: inherit;
    transition: border-color 0.1s;
  }
  .card:hover,
  .card:focus-visible {
    border-color: #f97316;
    outline: none;
  }
  .label {
    font-weight: 600;
    font-size: 1.1rem;
  }
  .meta {
    margin-top: 4px;
    color: #64748b;
    font-size: 0.85rem;
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .color-dot {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 50%;
    border: 1px solid rgba(0, 0, 0, 0.15);
  }
  .right {
    flex-shrink: 0;
  }
  .status {
    display: inline-block;
    padding: 4px 10px;
    border-radius: 999px;
    font-size: 0.85rem;
    font-weight: 600;
    background: #f1f5f9;
    color: #475569;
  }
  .card[data-status='top'] .status {
    background: #ecfdf5;
    color: #047857;
  }
  .card[data-status='zone'] .status {
    background: #fef3c7;
    color: #92400e;
  }
  .card[data-status='tries'] .status {
    background: #fef2f2;
    color: #b91c1c;
  }
</style>
