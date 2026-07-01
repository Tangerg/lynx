import type { WorkspaceEventLike } from "../domain/eventInvalidation";

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_CAP_MS = 30_000;
const RETARGET = Symbol("workspace-events.retarget");

export interface WorkspaceEventLoopDeps {
  subscribe(input: {
    cwd: string | undefined;
    signal: AbortSignal;
  }): Promise<AsyncIterable<WorkspaceEventLike>>;
  handleEvent(ev: WorkspaceEventLike): void;
  invalidateAll(): void;
  reportError(error: unknown): void;
}

export interface WorkspaceEventLoop {
  start(signal: AbortSignal): void;
  retarget(cwd: string | undefined): void;
}

export function createWorkspaceEventLoop(deps: WorkspaceEventLoopDeps): WorkspaceEventLoop {
  let watchCwd: string | undefined;
  let iterAbort: AbortController | null = null;

  return {
    start(signal) {
      void subscribeLoop(
        deps,
        signal,
        () => watchCwd,
        (next) => {
          iterAbort = next;
        },
      );
    },
    retarget(cwd) {
      if (cwd === undefined || cwd === watchCwd) return;
      watchCwd = cwd;
      iterAbort?.abort(RETARGET);
    },
  };
}

async function subscribeLoop(
  deps: WorkspaceEventLoopDeps,
  signal: AbortSignal,
  watchCwd: () => string | undefined,
  setIterAbort: (controller: AbortController | null) => void,
): Promise<void> {
  let attempt = 0;
  while (!signal.aborted) {
    const iter = new AbortController();
    setIterAbort(iter);
    const onOuterAbort = () => iter.abort();
    signal.addEventListener("abort", onOuterAbort, { once: true });
    try {
      const events = await deps.subscribe({ cwd: watchCwd(), signal: iter.signal });
      attempt = 0;
      deps.invalidateAll();
      for await (const ev of events) deps.handleEvent(ev);
    } catch (error) {
      if (!signal.aborted && iter.signal.reason !== RETARGET) deps.reportError(error);
    } finally {
      signal.removeEventListener("abort", onOuterAbort);
      setIterAbort(null);
    }
    if (signal.aborted) return;
    if (iter.signal.reason === RETARGET) {
      attempt = 0;
      continue;
    }
    await delay(Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_CAP_MS), signal);
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
