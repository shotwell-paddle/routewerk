<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { requestMagicLink, ApiClientError } from '$lib/api/client';
  import { authState, isAuthenticated } from '$lib/stores/auth.svelte';

  let email = $state('');
  let next = $state('');
  let submitting = $state(false);
  let submitted = $state(false);
  let error = $state<string | null>(null);

  // Read ?next= so /sign-in preserves the target route through the
  // magic-link round trip. /verify-magic is the server-side redirect
  // target after the user clicks the email link; it accepts the same
  // `next` query param.
  //
  // The page is the SPA's magic-link entry point. The HTMX /login
  // (password auth, used by staff today) is unchanged.
  onMount(() => {
    const params = new URLSearchParams(window.location.search);
    next = params.get('next') ?? '';

    // If we're already authenticated by the time we hit /sign-in,
    // skip the form and send the user where they were going.
    if (isAuthenticated()) {
      goto(next || '/');
    }
  });

  // If the layout's loadMe() resolves while we're sitting on /login,
  // bounce immediately so users don't keep seeing the form.
  $effect(() => {
    const a = authState();
    if (a.loaded && a.me !== null) {
      goto(next || '/');
    }
  });

  async function submit(e: Event) {
    e.preventDefault();
    if (submitting) return;
    error = null;
    submitting = true;
    try {
      await requestMagicLink({ email, next: next || undefined });
      submitted = true;
    } catch (err) {
      error = err instanceof ApiClientError ? err.message : 'Could not send the link. Try again.';
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>Sign in — Routewerk</title>
</svelte:head>

<main>
  <div class="card">
    <h1>Sign in</h1>

    {#if submitted}
      <p class="success">
        Check your inbox. We sent a sign-in link to <strong>{email}</strong>.
        It expires in 15 minutes.
      </p>
      <p class="hint">Wrong address? <button type="button" class="link" onclick={() => (submitted = false)}>Try again</button></p>
    {:else}
      <p class="hint">We'll email you a one-tap sign-in link. No password needed.</p>
      <form onsubmit={submit}>
        <label for="email">Email</label>
        <input
          id="email"
          type="email"
          required
          bind:value={email}
          placeholder="you@example.com"
          autocomplete="email"
        />
        {#if error}
          <p class="error">{error}</p>
        {/if}
        <button type="submit" disabled={submitting || !email}>
          {submitting ? 'Sending…' : 'Email me a link'}
        </button>
      </form>
    {/if}
  </div>
</main>

<style>
  main {
    min-height: 100dvh;
    display: grid;
    place-items: center;
    padding: 1rem;
    background: var(--rw-bg);
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    box-shadow: 0 1px 3px rgba(15, 20, 34, 0.06);
    padding: 2rem;
    width: 100%;
    max-width: 380px;
  }
  h1 {
    margin: 0 0 0.5rem;
    font-size: 1.5rem;
    font-weight: 700;
    letter-spacing: -0.01em;
  }
  .hint {
    color: var(--rw-text-muted);
    font-size: 0.95rem;
    margin: 0 0 1.5rem;
  }
  label {
    display: block;
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--rw-text-muted);
    margin-bottom: 0.4rem;
  }
  input[type='email'] {
    width: 100%;
    padding: 0.7rem 0.85rem;
    border: 1px solid var(--rw-border-strong);
    border-radius: 8px;
    font-size: 1rem;
    box-sizing: border-box;
    background: var(--rw-surface);
    color: var(--rw-text);
  }
  input[type='email']:focus {
    outline: none;
    border-color: var(--rw-accent);
  }
  button[type='submit'] {
    margin-top: 1rem;
    width: 100%;
    padding: 0.8rem 1rem;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    border: 1px solid var(--rw-accent);
    border-radius: 8px;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
  }
  button[type='submit']:hover:not(:disabled) {
    background: var(--rw-accent-hover);
    border-color: var(--rw-accent-hover);
  }
  button[type='submit']:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .success {
    color: #166534;
    background: rgba(22, 163, 74, 0.08);
    border: 1px solid rgba(22, 163, 74, 0.25);
    padding: 0.85rem;
    border-radius: 8px;
    margin: 0 0 1rem;
  }
  .error {
    color: #b91c1c;
    background: #fef2f2;
    border: 1px solid #fecaca;
    padding: 0.65rem 0.85rem;
    border-radius: 6px;
    margin: 0.75rem 0 0;
    font-size: 0.95rem;
  }
  .link {
    background: none;
    border: 0;
    color: var(--rw-text);
    text-decoration: underline;
    text-decoration-color: var(--rw-accent);
    text-underline-offset: 3px;
    cursor: pointer;
    padding: 0;
    font: inherit;
  }
</style>
