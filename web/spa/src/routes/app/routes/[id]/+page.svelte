<script lang="ts">
  import { page } from '$app/state';
  import PlaceholderPage from '$lib/components/PlaceholderPage.svelte';

  const id = $derived(page.params.id ?? '');
</script>

<PlaceholderPage title="Route detail" htmlPath="/routes/{id}" />
