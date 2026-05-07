<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import {
    getWall,
    updateWall,
    deleteWall,
    archiveWall,
    unarchiveWall,
    ApiClientError,
    type WallShape,
    type WallWriteShape,
  } from '$lib/api/client';
  import { effectiveLocationId } from '$lib/stores/location.svelte';
  import { roleRankAt } from '$lib/stores/auth.svelte';
  import WallForm from '$lib/components/WallForm.svelte';

  let wall = $state<WallShape | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);

  let editing = $state(false);
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let deleting = $state(false);
  let archiving = $state(false);

  const wallId = $derived(page.params.id ?? '');
  const locId = $derived(effectiveLocationId());
  const canEdit = $derived(roleRankAt(locId) >= 2);
  const canDelete = $derived(roleRankAt(locId) >= 3);

  $effect(() => {
    if (!locId || !wallId) return;
    let cancelled = false;
    loading = true;
    error = null;
    getWall(locId, wallId)
      .then((w) => {
        if (!cancelled) wall = w;
      })
      .catch((err) => {
        if (cancelled) return;
        error = err instanceof ApiClientError ? err.message : 'Could not load wall.';
      })
      .finally(() => {
        if (!cancelled) loading = false;
      });
    return () => {
      cancelled = true;
    };
  });

  async function submitEdit(body: WallWriteShape) {
    if (!locId || !wallId) return;
    saving = true;
    saveError = null;
    try {
      wall = await updateWall(locId, wallId, body);
      editing = false;
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Could not update wall.';
    } finally {
      saving = false;
    }
  }

  async function handleDelete() {
    if (!locId || !wallId || !wall) return;
    if (
      !confirm(
        `Permanently delete "${wall.name}"? Routes on this wall will need to be moved or deleted first.`,
      )
    )
      return;
    deleting = true;
    try {
      await deleteWall(locId, wallId);
      goto('/walls');
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Could not delete wall.';
      deleting = false;
    }
  }

  // Archive / unarchive — preferred to delete because it's reversible and
  // keeps route history. Mirrors POST /walls/{id}/archive on the HTMX side.
  async function toggleArchive() {
    if (!locId || !wallId || !wall) return;
    archiving = true;
    saveError = null;
    try {
      if (wall.archived_at) {
        wall = await unarchiveWall(locId, wallId);
      } else {
        wall = await archiveWall(locId, wallId);
      }
    } catch (err) {
      saveError = err instanceof ApiClientError ? err.message : 'Could not update archive state.';
    } finally {
      archiving = false;
    }
  }
</script>

<svelte:head>
  <title>{wall?.name ?? 'Wall'} — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/walls">← Walls</a>

  {#if loading}
    <p class="muted">Loading…</p>
  {:else if error}
    <p class="error">{error}</p>
  {:else if wall}
    <header class="page-header">
      <div>
        <span class="type-badge type-{wall.wall_type}">{wall.wall_type}</span>
        {#if wall.archived_at}<span class="archived-badge">archived</span>{/if}
        <h1>{wall.name}</h1>
      </div>
      {#if !editing && canEdit}
        <div class="header-actions">
          <button onclick={() => (editing = true)}>Edit</button>
        </div>
      {/if}
    </header>

    {#if editing}
      <WallForm
        initial={wall}
        submitLabel="Save changes"
        onSubmit={submitEdit}
        onCancel={() => {
          editing = false;
          saveError = null;
        }}
        {saving}
        error={saveError} />
    {:else}
      <section class="card details">
        <h2>Details</h2>
        <dl>
          <div><dt>Type</dt><dd>{wall.wall_type}</dd></div>
          {#if wall.angle}<div><dt>Angle</dt><dd>{wall.angle}</dd></div>{/if}
          {#if wall.height_meters != null}
            <div><dt>Height</dt><dd>{wall.height_meters.toFixed(1)} m</dd></div>
          {/if}
          {#if wall.num_anchors != null}
            <div><dt>Anchors</dt><dd>{wall.num_anchors}</dd></div>
          {/if}
          {#if wall.surface_type}
            <div><dt>Surface</dt><dd>{wall.surface_type}</dd></div>
          {/if}
          <div><dt>Sort order</dt><dd>{wall.sort_order}</dd></div>
        </dl>
      </section>

      <section class="card routes-stub">
        <h2>Routes</h2>
        <p class="muted">
          The route list for this wall will land in Phase 2.3.
          Use <a class="link" href="/walls/{wall.id}">the existing route view</a>
          for now.
        </p>
      </section>

      {#if canDelete}
        <section class="card archive-card">
          <h2>{wall.archived_at ? 'Restore' : 'Archive'}</h2>
          <p class="muted">
            {#if wall.archived_at}
              This wall is archived — climbers and setters don't see it.
              Restore to bring it back.
            {:else}
              Archive hides this wall from climbers + setters but keeps its
              route history intact. Reversible (unlike delete).
            {/if}
          </p>
          {#if saveError}<p class="error">{saveError}</p>{/if}
          <button disabled={archiving} onclick={toggleArchive}>
            {archiving
              ? '…'
              : wall.archived_at
                ? 'Restore wall'
                : 'Archive wall'}
          </button>
        </section>

        <section class="card danger-zone">
          <h2>Danger zone</h2>
          <p class="muted">
            Deletion is permanent. Prefer archive if you may want this wall
            back. Routes on this wall must be reassigned or deleted first.
          </p>
          <button class="danger" disabled={deleting} onclick={handleDelete}>
            {deleting ? 'Deleting…' : 'Delete wall'}
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
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0.25rem 0 0;
    letter-spacing: -0.01em;
  }
  .type-badge {
    font-size: 0.7rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    color: var(--rw-text-muted);
    background: var(--rw-surface-alt);
    display: inline-block;
  }
  .type-boulder {
    background: rgba(245, 158, 11, 0.15);
    color: #92590a;
  }
  .type-route {
    background: rgba(59, 130, 246, 0.12);
    color: #1d4ed8;
  }
  .header-actions {
    display: flex;
    gap: 8px;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .card h2 {
    font-size: 1.05rem;
    font-weight: 600;
    margin: 0 0 0.85rem;
  }
  .details dl {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 0.75rem 1.5rem;
  }
  .details dt {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--rw-text-faint);
    margin: 0 0 2px;
  }
  .details dd {
    margin: 0;
    color: var(--rw-text);
    font-weight: 500;
  }
  .archived-badge {
    display: inline-block;
    margin-left: 6px;
    background: var(--rw-surface-alt);
    color: var(--rw-text-muted);
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    padding: 2px 8px;
    border-radius: 4px;
    font-weight: 700;
  }
  .archive-card {
    border-color: rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.04);
  }
  .archive-card h2 {
    color: #92590a;
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
    padding: 0.5rem 1rem;
    border-radius: 6px;
    border: 1px solid var(--rw-border-strong);
    background: var(--rw-surface);
    color: var(--rw-text);
    font-size: 0.9rem;
    font-weight: 600;
  }
  button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  button.danger {
    background: #fff;
    color: #b91c1c;
    border-color: #fecaca;
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
    margin: 0 0 0.5rem;
  }
</style>
