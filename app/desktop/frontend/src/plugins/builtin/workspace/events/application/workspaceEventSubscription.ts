import type { WorkspaceEventLoop } from "./workspaceEventLoop";

const RESOLVE_RETRY_BASE_MS = 1_000;
const RESOLVE_RETRY_CAP_MS = 30_000;

export type WorkspaceCwdResolution =
  { status: "resolved"; cwd?: string } | { status: "unavailable" };

export interface WorkspaceEventSubscriptionPorts {
  canSubscribe: () => boolean;
  subscribeCapabilities: (onChange: () => void) => () => void;
  resolveWorkspaceCwd: () => Promise<WorkspaceCwdResolution>;
  subscribeWorkspaceCwdInputs: (onChange: () => void) => () => void;
  loop: WorkspaceEventLoop;
}

export function startWorkspaceEventSubscription(
  ports: WorkspaceEventSubscriptionPorts,
): () => void {
  const controller = new AbortController();
  let started = false;
  let retargetGeneration = 0;
  let retryTimer: ReturnType<typeof setTimeout> | undefined;

  const clearRetry = (): void => {
    if (retryTimer === undefined) return;
    clearTimeout(retryTimer);
    retryTimer = undefined;
  };

  const resolveTarget = (generation: number, attempt: number): void => {
    void ports.resolveWorkspaceCwd().then((resolution) => {
      if (generation !== retargetGeneration || controller.signal.aborted) return;
      if (resolution.status === "resolved") {
        ports.loop.retarget({
          type: "workspace",
          ...(resolution.cwd ? { cwd: resolution.cwd } : {}),
        });
        return;
      }
      retryTimer = setTimeout(
        () => resolveTarget(generation, attempt + 1),
        Math.min(RESOLVE_RETRY_BASE_MS * 2 ** attempt, RESOLVE_RETRY_CAP_MS),
      );
    });
  };

  const retarget = (): void => {
    const generation = ++retargetGeneration;
    clearRetry();
    // Do not let an unresolved active session inherit either the Runtime's
    // default workspace or the previous session's file watch. The global topic
    // stream remains online while the authoritative identity is retried.
    ports.loop.retarget({ type: "none" });
    resolveTarget(generation, 0);
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
    clearRetry();
    unsubscribeCapabilities();
    unsubscribeCwdInputs();
    controller.abort();
  };
}
