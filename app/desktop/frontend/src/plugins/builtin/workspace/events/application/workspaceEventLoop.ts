import type { WorkspaceEventLike } from "../domain/eventInvalidation";

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_CAP_MS = 30_000;
const RETARGET = Symbol("workspace-events.retarget");

export interface WorkspaceEventLoopDeps {
  subscribe(input: {
    target: WorkspaceWatchTarget;
    signal: AbortSignal;
  }): Promise<AsyncIterable<WorkspaceEventLike>>;
  handleEvent(ev: WorkspaceEventLike): void;
  invalidateAll(): void;
  reportDisconnect(error?: unknown): void;
}

export interface WorkspaceEventLoop {
  start(signal: AbortSignal): void;
  retarget(target: WorkspaceWatchTarget): void;
}

/**
 * `none` means the app-wide topics stay subscribed without a file watch while
 * active-session identity is unresolved. It is intentionally distinct from a
 * resolved workspace with no cwd, which means the Runtime's default workspace.
 */
export type WorkspaceWatchTarget = { type: "none" } | { type: "workspace"; cwd?: string };

function sameTarget(left: WorkspaceWatchTarget, right: WorkspaceWatchTarget): boolean {
  return (
    left.type === right.type &&
    (left.type === "none" || right.type === "none" || left.cwd === right.cwd)
  );
}

export function createWorkspaceEventLoop(deps: WorkspaceEventLoopDeps): WorkspaceEventLoop {
  let watchTarget: WorkspaceWatchTarget = { type: "none" };
  let iterAbort: AbortController | null = null;
  let generation = 0;

  return {
    start(signal) {
      const ownGeneration = ++generation;
      void subscribeLoop(
        deps,
        signal,
        () => watchTarget,
        (next) => {
          if (generation === ownGeneration) iterAbort = next;
        },
      );
    },
    retarget(target) {
      if (sameTarget(target, watchTarget)) return;
      watchTarget = target;
      iterAbort?.abort(RETARGET);
    },
  };
}

async function subscribeLoop(
  deps: WorkspaceEventLoopDeps,
  signal: AbortSignal,
  watchTarget: () => WorkspaceWatchTarget,
  setIterAbort: (controller: AbortController | null) => void,
): Promise<void> {
  let attempt = 0;
  while (!signal.aborted) {
    const iter = new AbortController();
    setIterAbort(iter);
    const onOuterAbort = () => iter.abort();
    signal.addEventListener("abort", onOuterAbort, { once: true });
    let failure: unknown;
    try {
      const events = await deps.subscribe({ target: watchTarget(), signal: iter.signal });
      // A transport may resolve its opening promise at the same instant a
      // retarget abort wins. Do not publish the stale subscription's initial
      // resync or any already-buffered event into the new workspace target.
      if (iter.signal.aborted) continue;
      attempt = 0;
      deps.invalidateAll();
      let lastSequence = 0;
      for await (const ev of events) {
        if (iter.signal.aborted) break;
        if (ev.sequence !== lastSequence + 1) {
          deps.invalidateAll();
        }
        lastSequence = ev.sequence;
        deps.handleEvent(ev);
      }
    } catch (error) {
      if (!signal.aborted && iter.signal.reason !== RETARGET) failure = error;
    } finally {
      signal.removeEventListener("abort", onOuterAbort);
      setIterAbort(null);
    }
    if (signal.aborted) return;
    if (iter.signal.reason === RETARGET) {
      attempt = 0;
      continue;
    }
    // An RPC stream ending without outer cancellation is also a connection
    // signal. Let the Runtime context verify and withdraw capabilities promptly
    // instead of allowing this consumer to guess global connection health.
    deps.reportDisconnect(failure);
    const backoff = new AbortController();
    setIterAbort(backoff);
    const abortBackoff = () => backoff.abort();
    signal.addEventListener("abort", abortBackoff, { once: true });
    await delay(Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_CAP_MS), backoff.signal);
    signal.removeEventListener("abort", abortBackoff);
    setIterAbort(null);
    if (signal.aborted) return;
    if (backoff.signal.reason === RETARGET) {
      attempt = 0;
      continue;
    }
    attempt += 1;
  }
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(done, ms);
    function done(): void {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    }
    signal.addEventListener("abort", done, { once: true });
  });
}
