import type { WorkspaceEventLoop } from "./workspaceEventLoop";

export type WorkspaceCwdResolution =
  { status: "resolved"; cwd?: string } | { status: "unavailable" };

export interface WorkspaceEventSubscriptionPorts {
  canSubscribe: () => boolean;
  subscribeCapabilities: (onChange: () => void) => () => void;
  resolveWorkspaceCwd: (signal: AbortSignal) => Promise<WorkspaceCwdResolution>;
  reportResolutionError: (error: unknown) => void;
  subscribeWorkspaceCwdInputs: (onChange: () => void) => () => void;
  loop: WorkspaceEventLoop;
}

const RESOLVE_RETRY_BASE_MS = 1_000;
const RESOLVE_RETRY_CAP_MS = 30_000;

export function startWorkspaceEventSubscription(
  ports: WorkspaceEventSubscriptionPorts,
): () => void {
  const controller = new AbortController();
  let started = false;
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

  const retarget = (): void => {
    const generation = ++retargetGeneration;
    // Do not let an unresolved active session inherit either the Runtime's
    // default workspace or the previous session's file watch. The global topic
    // stream remains online until the session projection changes and supplies
    // another authoritative identity input.
    ports.loop.retarget({ type: "none" });
    resolveTarget(generation);
  };

  const startIfAdvertised = (): void => {
    if (started || controller.signal.aborted || !ports.canSubscribe()) return;
    started = true;
    ports.loop.start(controller.signal);
  };

  startIfAdvertised();
  const unsubscribeCapabilities = ports.subscribeCapabilities(startIfAdvertised);
  retarget();
  const unsubscribeCwdInputs = ports.subscribeWorkspaceCwdInputs(retarget);

  return () => {
    retargetGeneration += 1;
    resolutionAbort?.abort();
    unsubscribeCapabilities();
    unsubscribeCwdInputs();
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
