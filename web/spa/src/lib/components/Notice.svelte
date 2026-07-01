<script lang="ts">
  import type { Snippet } from 'svelte';

  // Inline status/error notice with a proper ARIA live region so screen
  // readers announce the message when it appears. Errors are assertive
  // (role="alert"); ok/loading are polite (role="status"). Styles mirror
  // the .error / .ok blocks the pages already use.
  let {
    kind,
    children,
  }: {
    kind: 'error' | 'ok' | 'loading';
    children: Snippet;
  } = $props();
</script>

<p class="notice {kind}" role={kind === 'error' ? 'alert' : 'status'}>
  {@render children()}
</p>

<style>
  .notice {
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.5rem 0;
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
  }
  .ok {
    background: rgba(22, 163, 74, 0.1);
    border: 1px solid rgba(22, 163, 74, 0.3);
    color: #15803d;
  }
  .loading {
    padding: 0;
    color: var(--rw-text-muted);
  }
</style>
