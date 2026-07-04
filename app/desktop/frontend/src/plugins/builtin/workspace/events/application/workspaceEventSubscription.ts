import type { WorkspaceEventLoop } from "./workspaceEventLoop";

export interface WorkspaceEventSubscriptionPorts {
  canSubscribe: () => boolean;
  subscribeCapabilities: (onChange: () => void) => () => void;
  resolveWorkspaceCwd: () => Promise<string | undefined>;
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
    void ports.resolveWorkspaceCwd().then((cwd) => {
      if (generation !== retargetGeneration || controller.signal.aborted) return;
      ports.loop.retarget(cwd);
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
