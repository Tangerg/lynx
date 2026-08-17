import type { WorkspaceEventLoop } from "./workspaceEventLoop";

export type WorkspaceCwdResolution =
  { status: "resolved"; cwd?: string } | { status: "unavailable" };

export interface WorkspaceEventSubscriptionPorts {
  canSubscribe: () => boolean;
  connectionGeneration: () => string | null;
  subscribeConnection: (onChange: () => void) => () => void;
  retireReadModels: () => void;
  resolveWorkspaceCwd: (signal: AbortSignal) => Promise<WorkspaceCwdResolution>;
  reportResolutionError: (error: unknown) => void;
  subscribeWorkspaceCwdInputs: (onChange: (change: WorkspaceCwdInputChange) => void) => () => void;
  loop: WorkspaceEventLoop;
}

class RuntimeEventLoopOwner {
  #observedConnection = false;
  #connectionGeneration: string | null = null;
  #abort: AbortController | null = null;

  constructor(
    private readonly loop: WorkspaceEventLoop,
    private readonly retireReadModels: () => void,
  ) {}

  reconcile(connectionGeneration: string | null, canSubscribe: boolean): void {
    const generationChanged =
      this.#observedConnection && this.#connectionGeneration !== connectionGeneration;
    const shouldStream = connectionGeneration !== null && canSubscribe;
    if (this.#observedConnection && !generationChanged && (this.#abort !== null) === shouldStream)
      return;

    // Runtime identity owns every read admitted through its connection, not
    // only runtime.subscribe. Revoke those writers synchronously before the
    // old tail is aborted or the successor tail begins opening. The successor
    // snapshot remains deliberately deferred until that tail is established.
    if (generationChanged) this.retireReadModels();
    this.#abort?.abort();
    this.#abort = null;
    this.#observedConnection = true;
    this.#connectionGeneration = connectionGeneration;
    if (!shouldStream) return;

    const abort = new AbortController();
    this.#abort = abort;
    void this.loop.start(abort.signal, connectionGeneration);
  }

  dispose(): void {
    this.#abort?.abort();
    this.#abort = null;
  }
}

export type WorkspaceCwdInputChange = "identity" | "projection";

const RESOLVE_RETRY_BASE_MS = 1_000;
const RESOLVE_RETRY_CAP_MS = 30_000;

export function startWorkspaceEventSubscription(
  ports: WorkspaceEventSubscriptionPorts,
): () => void {
  const controller = new AbortController();
  const eventLoop = new RuntimeEventLoopOwner(ports.loop, ports.retireReadModels);
  let retargetGeneration = 0;
  let resolutionAbort: AbortController | null = null;

  const resolveTarget = (generation: number): void => {
    resolutionAbort?.abort();
    const attemptAbort = new AbortController();
    resolutionAbort = attemptAbort;
    void (async () => {
      let attempt = 0;
      while (!attemptAbort.signal.aborted && !controller.signal.aborted) {
        try {
          const resolution = await ports.resolveWorkspaceCwd(attemptAbort.signal);
          if (
            generation !== retargetGeneration ||
            attemptAbort.signal.aborted ||
            controller.signal.aborted
          )
            return;
          if (resolution.status === "resolved") {
            ports.loop.retarget({
              type: "workspace",
              ...(resolution.cwd ? { cwd: resolution.cwd } : {}),
            });
          } else {
            ports.loop.retarget({ type: "none" });
          }
          return;
        } catch (error) {
          if (
            generation !== retargetGeneration ||
            attemptAbort.signal.aborted ||
            controller.signal.aborted
          )
            return;
          ports.reportResolutionError(error);
          await retryDelay(
            Math.min(RESOLVE_RETRY_BASE_MS * 2 ** attempt, RESOLVE_RETRY_CAP_MS),
            attemptAbort.signal,
          );
          attempt += 1;
        }
      }
    })();
  };

  const retarget = (change: WorkspaceCwdInputChange): void => {
    const generation = ++retargetGeneration;
    // A new active Session must never inherit the previous Session's watch while
    // its identity resolves. A projection update belongs to the same Session,
    // so keep the current watch until its workspace resolves: WorkspaceEventLoop
    // suppresses an equal target and avoids tearing down a healthy stream merely
    // because the Session list caught up after a cold direct read.
    if (change === "identity") ports.loop.retarget({ type: "none" });
    resolveTarget(generation);
  };

  const reconcileConnection = (): void => {
    if (controller.signal.aborted) return;
    eventLoop.reconcile(ports.connectionGeneration(), ports.canSubscribe());
  };

  reconcileConnection();
  const unsubscribeConnection = ports.subscribeConnection(reconcileConnection);
  retarget("identity");
  const unsubscribeCwdInputs = ports.subscribeWorkspaceCwdInputs(retarget);

  return () => {
    retargetGeneration += 1;
    resolutionAbort?.abort();
    unsubscribeConnection();
    unsubscribeCwdInputs();
    eventLoop.dispose();
    controller.abort();
  };
}

function retryDelay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timer = setTimeout(done, ms);
    function done(): void {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    }
    signal.addEventListener("abort", done, { once: true });
  });
}
