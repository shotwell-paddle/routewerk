<script lang="ts">
  let { title, htmlPath }: { title: string; htmlPath?: string } = $props();
</script>

<svelte:head>
  <title>{title} — Routewerk</title>
</svelte:head>

<div class="page">
  <h1>{title}</h1>
  <p class="lede">
    This surface hasn't migrated to the new app shell yet.
  </p>
  {#if htmlPath}
    <p>
      The existing version is still available at
      <a class="link" href={htmlPath}>{htmlPath}</a>.
    </p>
  {/if}
</div>

<style>
  .page {
    max-width: 48rem;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 0.5rem;
    letter-spacing: -0.01em;
  }
  .lede {
    color: var(--rw-text-muted);
    margin: 0 0 0.75rem;
  }
  .link {
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
  }
</style>
