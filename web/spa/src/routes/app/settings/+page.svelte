<script lang="ts">
  import {
    updateMe,
    changePassword,
    ApiClientError,
    type UpdateMeShape,
  } from '$lib/api/client';
  import { authState, currentUser, loadMe, roleRankAt } from '$lib/stores/auth.svelte';
  import { effectiveLocationId } from '$lib/stores/location.svelte';

  // Profile form is seeded once /me is loaded. We watch authState() and
  // re-seed if it changes (e.g. user just signed in).
  let profileForm = $state({
    display_name: '',
    avatar_url: '',
    bio: '',
  });
  let profileSeeded = $state(false);
  let profileSaving = $state(false);
  let profileError = $state<string | null>(null);
  let profileOk = $state<string | null>(null);

  $effect(() => {
    const a = authState();
    if (a.loaded && a.me && !profileSeeded) {
      profileForm = {
        display_name: a.me.user.display_name,
        avatar_url: a.me.user.avatar_url ?? '',
        bio: a.me.user.bio ?? '',
      };
      profileSeeded = true;
    }
  });

  async function saveProfile(e: Event) {
    e.preventDefault();
    profileSaving = true;
    profileError = null;
    profileOk = null;
    const me = currentUser();
    if (!me) {
      profileError = 'Not signed in.';
      profileSaving = false;
      return;
    }

    const body: UpdateMeShape = {};
    const trimmedName = profileForm.display_name.trim();
    if (!trimmedName) {
      profileError = 'Display name cannot be empty.';
      profileSaving = false;
      return;
    }
    if (trimmedName !== me.display_name) body.display_name = trimmedName;

    const trimmedAvatar = profileForm.avatar_url.trim();
    const currentAvatar = me.avatar_url ?? '';
    if (trimmedAvatar !== currentAvatar) {
      if (trimmedAvatar) body.avatar_url = trimmedAvatar;
      else body.clear_avatar_url = true;
    }

    const trimmedBio = profileForm.bio.trim();
    const currentBio = me.bio ?? '';
    if (trimmedBio !== currentBio) {
      if (trimmedBio) body.bio = trimmedBio;
      else body.clear_bio = true;
    }

    if (Object.keys(body).length === 0) {
      profileOk = 'Nothing to save.';
      profileSaving = false;
      return;
    }

    try {
      await updateMe(body);
      // Refresh the global auth store so the sidebar + other pages see
      // the new display_name immediately.
      await loadMe();
      profileOk = 'Saved.';
    } catch (err) {
      profileError = err instanceof ApiClientError ? err.message : 'Save failed.';
    } finally {
      profileSaving = false;
    }
  }

  // Password form
  let pwForm = $state({ old: '', new: '', confirm: '' });
  let pwSaving = $state(false);
  let pwError = $state<string | null>(null);
  let pwOk = $state<string | null>(null);

  async function savePassword(e: Event) {
    e.preventDefault();
    pwSaving = true;
    pwError = null;
    pwOk = null;
    if (!pwForm.old || !pwForm.new) {
      pwError = 'Both fields are required.';
      pwSaving = false;
      return;
    }
    if (pwForm.new.length < 8) {
      pwError = 'New password must be at least 8 characters.';
      pwSaving = false;
      return;
    }
    if (pwForm.new !== pwForm.confirm) {
      pwError = "New passwords don't match.";
      pwSaving = false;
      return;
    }
    try {
      await changePassword(pwForm.old, pwForm.new);
      pwOk = 'Password changed. Other sessions will need to sign in again.';
      pwForm = { old: '', new: '', confirm: '' };
    } catch (err) {
      pwError = err instanceof ApiClientError ? err.message : 'Password change failed.';
    } finally {
      pwSaving = false;
    }
  }
</script>

<svelte:head>
  <title>Settings — Routewerk</title>
</svelte:head>

<div class="page">
  <a class="back" href="/app/profile">← Profile</a>
  <h1>Settings</h1>

  {#if roleRankAt(effectiveLocationId()) >= 3}
    <section class="card cta-card">
      <h2>Gym settings</h2>
      <p class="muted">
        Circuits, hold colors, grading defaults, and what climbers see on
        cards. Per-location.
      </p>
      <a class="primary-link" href="/app/settings/gym">Manage gym settings →</a>
    </section>
  {/if}

  {#if roleRankAt(effectiveLocationId()) >= 3}
    <section class="card cta-card">
      <h2>Progressions admin</h2>
      <p class="muted">
        Quests, badges, and quest domains. Build out the catalog before
        flipping the gym setting that exposes them to climbers.
      </p>
      <a class="primary-link" href="/app/settings/progressions">Manage progressions →</a>
    </section>
  {/if}

  {#if roleRankAt(effectiveLocationId()) >= 3}
    <section class="card cta-card">
      <h2>Setter playbook</h2>
      <p class="muted">
        Default checklist applied to every new session. Reorder, rename,
        and prune steps so the team has a consistent shop-day cadence.
      </p>
      <a class="primary-link" href="/app/settings/playbook">Edit playbook →</a>
    </section>
  {/if}

  {#if roleRankAt(effectiveLocationId()) >= 5}
    <section class="card cta-card">
      <h2>Organization</h2>
      <p class="muted">
        Org-wide settings, locations, and the team admin. Org admin only.
      </p>
      <a class="primary-link" href="/app/settings/org">Manage organization →</a>
    </section>
  {/if}

  <section class="card">
    <h2>Profile</h2>
    <form onsubmit={saveProfile}>
      <div class="field">
        <label for="s-name">Display name *</label>
        <input id="s-name" bind:value={profileForm.display_name} />
      </div>
      <div class="field">
        <label for="s-avatar">Avatar URL</label>
        <input id="s-avatar" type="url" bind:value={profileForm.avatar_url} placeholder="https://…" />
      </div>
      <div class="field">
        <label for="s-bio">Bio</label>
        <textarea id="s-bio" bind:value={profileForm.bio} rows="3" placeholder="A short description for other climbers."></textarea>
      </div>
      {#if profileError}<p class="error">{profileError}</p>{/if}
      {#if profileOk}<p class="ok">{profileOk}</p>{/if}
      <div class="actions">
        <button class="primary" type="submit" disabled={profileSaving}>
          {profileSaving ? 'Saving…' : 'Save profile'}
        </button>
      </div>
    </form>
  </section>

  <section class="card">
    <h2>Change password</h2>
    <form onsubmit={savePassword}>
      <div class="field">
        <label for="p-old">Current password</label>
        <input id="p-old" type="password" autocomplete="current-password" bind:value={pwForm.old} />
      </div>
      <div class="field">
        <label for="p-new">New password</label>
        <input id="p-new" type="password" autocomplete="new-password" bind:value={pwForm.new} />
      </div>
      <div class="field">
        <label for="p-confirm">Confirm new password</label>
        <input id="p-confirm" type="password" autocomplete="new-password" bind:value={pwForm.confirm} />
      </div>
      {#if pwError}<p class="error">{pwError}</p>{/if}
      {#if pwOk}<p class="ok">{pwOk}</p>{/if}
      <p class="hint muted">
        Changing your password signs out other devices but keeps you signed in here.
      </p>
      <div class="actions">
        <button class="primary" type="submit" disabled={pwSaving}>
          {pwSaving ? 'Saving…' : 'Change password'}
        </button>
      </div>
    </form>
  </section>
</div>

<style>
  .page {
    max-width: 40rem;
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
  h1 {
    font-size: 1.6rem;
    font-weight: 700;
    margin: 0 0 1.25rem;
    letter-spacing: -0.01em;
  }
  h2 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0 0 0.75rem;
  }
  .card {
    background: var(--rw-surface);
    border: 1px solid var(--rw-border);
    border-radius: 12px;
    padding: 1.25rem;
    margin-bottom: 1rem;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
    margin-bottom: 0.85rem;
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
    margin-top: 0.5rem;
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
  .cta-card {
    border-color: var(--rw-accent);
  }
  .primary-link {
    display: inline-block;
    background: var(--rw-accent);
    color: var(--rw-accent-ink);
    padding: 0.55rem 1rem;
    border-radius: 8px;
    text-decoration: none;
    font-weight: 600;
    font-size: 0.9rem;
    border: 1px solid var(--rw-accent);
    margin-top: 0.5rem;
  }
  .primary-link:hover {
    background: var(--rw-accent-hover);
  }
  .hint {
    font-size: 0.8rem;
    margin: 0.5rem 0;
  }
  .muted {
    color: var(--rw-text-muted);
  }
  .error {
    background: #fef2f2;
    border: 1px solid #fecaca;
    color: #991b1b;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.5rem 0;
  }
  .ok {
    background: rgba(22, 163, 74, 0.1);
    border: 1px solid rgba(22, 163, 74, 0.3);
    color: #15803d;
    padding: 0.55rem 0.75rem;
    border-radius: 6px;
    font-size: 0.9rem;
    margin: 0.5rem 0;
  }
</style>
