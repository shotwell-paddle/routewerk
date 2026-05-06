// ActionQueue — climber-side scorecard state + optimistic mutation
// pipeline. One instance per (competition, registration) — the
// scorecard page constructs it on mount.
//
// Flow per tap:
//   1. Compute optimistic new state locally (mirrors server's
//      computeNewState in internal/handler/comp_action.go).
//   2. Render the new state immediately (~16ms).
//   3. POST /api/v1/competitions/{id}/actions with a UUIDv4
//      idempotency_key.
//   4a. On success: apply the server's authoritative state (in case
//       the server diverged — e.g. it auto-set zone when we tapped top).
//   4b. On rejection: revert to the pre-tap state, surface the reason.
//   4c. On network error: revert, surface error, allow user to re-tap
//       (the UUIDv4 key would dedupe the retry on the server side).
//
// Undo is a special case: we can't compute the new state locally
// because it depends on log history. The store shows a transient
// "loading" indicator and waits for the server's response.
//
// `.svelte.ts` extension is required for $state to work outside
// .svelte components (Svelte 5 rune contract).

import {
  submitActions,
  type ActionItem,
  type ActionType,
  type AttemptState,
  type ActionsResponse,
  ApiClientError,
} from '$lib/api/client';

interface QueueState {
  /** Per-problem current state (what the UI renders). */
  attempts: Record<string, AttemptState>;
  /** Per-problem error message; cleared on next successful tap. */
  errors: Record<string, string | null>;
  /** Per-problem in-flight action count (0 = idle). */
  pending: Record<string, number>;
}

export class ActionQueue {
  private state: QueueState = $state({
    attempts: {},
    errors: {},
    pending: {},
  });

  constructor(public readonly competitionId: string) {}

  /** Read-only access for components. */
  get attempts() {
    return this.state.attempts;
  }
  get errors() {
    return this.state.errors;
  }
  /** True if any action is in flight for the given problem. */
  isPending(problemId: string): boolean {
    return (this.state.pending[problemId] ?? 0) > 0;
  }

  /** Initialize from server-provided current state (page load). */
  hydrate(initial: AttemptState[]) {
    for (const s of initial) {
      this.state.attempts[s.problem_id] = { ...s };
    }
  }

  /**
   * Submit a tap. Returns once the server has responded (or errored).
   * Components don't need to await — fire-and-forget is fine, the
   * store updates reactively.
   */
  async submit(problemId: string, type: ActionType): Promise<void> {
    const before = this.currentOrZero(problemId);
    this.state.errors[problemId] = null;

    // For non-undo actions, paint the optimistic state immediately so
    // the climber sees instant feedback. Undo waits on the server.
    if (type !== 'undo') {
      this.state.attempts[problemId] = computeNewStateLocal(before, type);
    }

    this.state.pending[problemId] = (this.state.pending[problemId] ?? 0) + 1;

    const action: ActionItem = {
      idempotency_key: crypto.randomUUID() as ActionItem['idempotency_key'],
      problem_id: problemId as ActionItem['problem_id'],
      type,
      client_timestamp: new Date().toISOString(),
    };

    try {
      const resp = await submitActions(this.competitionId, { actions: [action] });
      this.applyResponse(action, before, resp);
    } catch (err) {
      // Network error / 5xx — revert and surface. The caller can re-tap;
      // the server's idempotency dedupe means a duplicate submission
      // (with the same key) won't double-apply.
      this.state.attempts[problemId] = before;
      this.state.errors[problemId] =
        err instanceof ApiClientError ? err.message : 'Network error — try again';
    } finally {
      this.state.pending[problemId] = (this.state.pending[problemId] ?? 1) - 1;
    }
  }

  // ── Internal ────────────────────────────────────────────

  private currentOrZero(problemId: string): AttemptState {
    return (
      this.state.attempts[problemId] ?? {
        problem_id: problemId as AttemptState['problem_id'],
        attempts: 0,
        zone_reached: false,
        top_reached: false,
      }
    );
  }

  private applyResponse(
    action: ActionItem,
    before: AttemptState,
    resp: ActionsResponse,
  ) {
    const problemId = action.problem_id as string;

    // Rejection? Revert and surface a friendly reason.
    const rej = resp.rejected.find((r) => r.idempotency_key === action.idempotency_key);
    if (rej) {
      this.state.attempts[problemId] = before;
      this.state.errors[problemId] = friendlyRejection(rej.reason);
      return;
    }

    // Apply the server's authoritative state for this problem. This
    // matters for undo (we couldn't compute locally) and for any
    // edge cases where server logic diverges from our local mirror
    // (e.g. server-side coercion on top auto-marking zone).
    const serverState = resp.state.find((s) => s.problem_id === action.problem_id);
    if (serverState) {
      this.state.attempts[problemId] = serverState;
    }
  }
}

// ── Pure: optimistic state computation ─────────────────────

/**
 * Mirrors `computeNewState` in internal/handler/comp_action.go.
 *
 * Keep this in lockstep with the server logic — divergence shows up
 * as "tap target says X, then snaps to Y after the server responds,"
 * which feels broken even though it's technically eventually consistent.
 *
 * Undo isn't implementable here (depends on log history); the store
 * routes around this by leaving local state untouched and waiting on
 * the server for undo.
 */
export function computeNewStateLocal(current: AttemptState, type: ActionType): AttemptState {
  switch (type) {
    case 'increment':
      return { ...current, attempts: current.attempts + 1 };

    case 'zone': {
      const attempts = Math.max(current.attempts, 1);
      return {
        ...current,
        attempts,
        zone_reached: true,
        zone_attempts: attempts,
      };
    }

    case 'top': {
      const attempts = Math.max(current.attempts, 1);
      const next: AttemptState = {
        ...current,
        attempts,
        top_reached: true,
      };
      // Top implies zone (you climbed past it). Set zone_reached + zone_attempts
      // if not already, matching the server's coercion.
      if (!current.zone_reached) {
        next.zone_reached = true;
        next.zone_attempts = attempts;
      }
      return next;
    }

    case 'reset':
      return {
        problem_id: current.problem_id,
        attempts: 0,
        zone_reached: false,
        top_reached: false,
      };

    case 'undo':
      throw new Error('undo cannot be computed locally');

    default:
      throw new Error(`unknown action type: ${String(type)}`);
  }
}

/** Map server reason codes to user-friendly messages. */
function friendlyRejection(reason: string): string {
  switch (reason) {
    case 'event_closed':
      return 'This event has ended.';
    case 'unknown_problem':
      return 'That problem no longer exists.';
    case 'wrong_competition':
      return "That problem isn't part of this competition.";
    case 'no_history':
      return 'Nothing to undo yet.';
    case 'server_error':
      return 'Something went wrong on the server.';
    default:
      return 'Action could not be applied.';
  }
}
