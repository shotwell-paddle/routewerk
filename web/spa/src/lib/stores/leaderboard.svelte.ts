// LeaderboardStore — subscribes to the comp's SSE stream and exposes
// the latest leaderboard payload as reactive Svelte state.
//
// EventSource is the right primitive here: native browser API, includes
// cookies on same-origin requests (matches our auth model), built-in
// reconnect with exponential backoff. The server (handler/comp_sse.go)
// emits a `leaderboard` event on connect (initial snapshot) and after
// every action / verify / override, so this store stays current with
// no polling.
//
// Lifecycle: caller constructs in onMount with the comp ID, calls
// connect() once, and close() on unmount. The store handles reconnects
// internally via EventSource's built-in retry; we surface the
// disconnected state in `connected` so the UI can show a subtle dot.

import type { Leaderboard } from '$lib/api/client';

interface LeaderboardStoreState {
  /** Latest leaderboard frame, or null until the first event arrives. */
  board: Leaderboard | null;
  /** True between successful open and any error event. */
  connected: boolean;
  /** Last error from the EventSource, if any. */
  error: string | null;
}

export class LeaderboardStore {
  private state: LeaderboardStoreState = $state({
    board: null,
    connected: false,
    error: null,
  });
  private es: EventSource | null = null;

  constructor(
    public readonly competitionId: string,
    public readonly categoryId?: string,
  ) {}

  get board() {
    return this.state.board;
  }
  get connected() {
    return this.state.connected;
  }
  get error() {
    return this.state.error;
  }

  /**
   * Open the SSE connection. Safe to call once per store instance —
   * subsequent calls are no-ops. EventSource handles reconnect on its
   * own (with exponential backoff per the spec).
   */
  connect() {
    if (this.es) return;

    const params = new URLSearchParams();
    if (this.categoryId) params.set('category', this.categoryId);
    const qs = params.toString();
    const url =
      `/api/v1/competitions/${this.competitionId}/leaderboard/stream` +
      (qs ? `?${qs}` : '');

    // EventSource constructor doesn't take a credentials option; it
    // sends cookies on same-origin by default, which is exactly what
    // our cookie-or-JWT middleware needs.
    this.es = new EventSource(url);

    this.es.addEventListener('open', () => {
      this.state.connected = true;
      this.state.error = null;
    });

    this.es.addEventListener('leaderboard', (ev: MessageEvent<string>) => {
      try {
        this.state.board = JSON.parse(ev.data) as Leaderboard;
      } catch (err) {
        // Malformed frame — surface but don't disconnect; next frame
        // will probably parse fine.
        this.state.error = err instanceof Error ? err.message : 'Bad SSE frame';
      }
    });

    this.es.addEventListener('error', () => {
      // EventSource fires `error` both on transient blips and on
      // hard failures (CONNECTING vs CLOSED states distinguish them).
      // We surface "disconnected" only if the connection is fully
      // closed; transient retries don't need to scare the UI.
      this.state.connected = false;
      if (this.es?.readyState === EventSource.CLOSED) {
        this.state.error = 'connection closed';
      }
    });
  }

  /** Tear down the connection. Idempotent. */
  close() {
    if (!this.es) return;
    this.es.close();
    this.es = null;
    this.state.connected = false;
  }
}
