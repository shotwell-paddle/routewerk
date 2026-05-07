<script lang="ts">
  import type { SessionShape, SessionWriteShape } from '$lib/api/client';

  let {
    initial,
    submitLabel,
    onSubmit,
    onCancel,
    saving = false,
    error = null,
  }: {
    initial?: SessionShape | null;
    submitLabel: string;
    onSubmit: (body: SessionWriteShape) => void | Promise<void>;
    onCancel?: () => void;
    saving?: boolean;
    error?: string | null;
  } = $props();

  // svelte-ignore state_referenced_locally
  const seed = initial;

  let form = $state({
    scheduled_date: (seed?.scheduled_date ?? new Date().toISOString()).slice(0, 10),
    notes: seed?.notes ?? '',
  });
  let localError = $state<string | null>(null);

  function handleSubmit(e: Event) {
    e.preventDefault();
    localError = null;
    if (!form.scheduled_date) {
      localError = 'Scheduled date is required.';
      return;
    }
    onSubmit({
      scheduled_date: form.scheduled_date,
      notes: form.notes.trim() || null,
    });
  }
</script>

<form class="form" onsubmit={handleSubmit}>
  <div class="field">
    <label for="s-date">Scheduled date *</label>
    <input id="s-date" type="date" bind:value={form.scheduled_date} />
  </div>

  <div class="field">
    <label for="s-notes">Notes</label>
    <textarea
      id="s-notes"
      bind:value={form.notes}
      rows="4"
      placeholder="Goals, focus areas, anything the team should know…"></textarea>
  </div>

  {#if localError || error}
    <p class="error">{localError ?? error}</p>
  {/if}

  <div class="actions">
    <button type="submit" class="primary" disabled={saving}>
      {saving ? 'Saving…' : submitLabel}
    </button>
    {#if onCancel}
      <button type="button" onclick={onCancel} disabled={saving}>Cancel</button>
    {/if}
  </div>
</form>

<style>
  .form {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.85rem;
    max-width: 36rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field label {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--rw-text-muted);
  }
  .field input,
  .field textarea {
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 6px;
    font-size: 0.95rem;
    background: var(--rw-surface);
    color: var(--rw-text);
    box-sizing: border-box;
    font-family: inherit;
  }
  .field input:focus,
  .field textarea:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  .actions {
    display: flex;
    gap: 8px;
    margin-top: 0.25rem;
  }
  button {
    cursor: pointer;
    padding: 0.55rem 1rem;
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
  button.primary {
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border-color: var(--rw-accent);
  }
  button.primary:hover:not(:disabled) {
    background: var(--rw-accent-hover);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0;
  }
</style>
