import type { WorkspaceEventLoop } from "./workspaceEventLoop";

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

  const retarget = (): void => {
    const generation = ++retargetGeneration;
    void ports.resolveWorkspaceCwd().then((resolution) => {
      if (generation !== retargetGeneration || controller.signal.aborted) return;
      if (resolution.status === "resolved") ports.loop.retarget(resolution.cwd);
    });
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
    unsubscribeCapabilities();
    unsubscribeCwdInputs();
    controller.abort();
  };
}
