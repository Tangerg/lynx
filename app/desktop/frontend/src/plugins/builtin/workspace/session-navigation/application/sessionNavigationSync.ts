import type {
  AgentSessionLifecycleSnapshot,
  AgentSessionSelectionSnapshot,
} from "@/plugins/builtin/agent/public/session";

export type AgentSessionSelectionListener = (
  state: AgentSessionSelectionSnapshot,
  previous: AgentSessionSelectionSnapshot,
) => void;
export type AgentSessionLifecycleListener = (state: AgentSessionLifecycleSnapshot) => void;

export interface WorkspaceSessionNavigationPorts {
  activeSessionId: () => string;
  lifecycleSnapshot: () => AgentSessionLifecycleSnapshot;
  subscribeSelection: (listener: AgentSessionSelectionListener) => () => void;
  subscribeLifecycle: (listener: AgentSessionLifecycleListener) => () => void;
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
  selectChat: () => void;
}

export function syncWorkspaceSessionSelection(
  state: AgentSessionSelectionSnapshot,
  previous: AgentSessionSelectionSnapshot,
  ports: Pick<WorkspaceSessionNavigationPorts, "activateSessionScope" | "selectChat">,
): void {
  if (state.selectionEpoch !== previous.selectionEpoch) ports.selectChat();
  if (state.activeSessionId !== previous.activeSessionId) {
    ports.activateSessionScope(state.activeSessionId);
  }
}

export function syncWorkspaceSessionLifecycle(
  state: AgentSessionLifecycleSnapshot,
  ports: Pick<WorkspaceSessionNavigationPorts, "forgetSessionScopes">,
): void {
  ports.forgetSessionScopes(state.openSessionIds);
}

export function bindWorkspaceSessionNavigation(ports: WorkspaceSessionNavigationPorts): () => void {
  ports.activateSessionScope(ports.activeSessionId());
  ports.forgetSessionScopes(ports.lifecycleSnapshot().openSessionIds);

  const unsubscribeSelection = ports.subscribeSelection((state, previous) => {
    syncWorkspaceSessionSelection(state, previous, ports);
  });
  const unsubscribeLifecycle = ports.subscribeLifecycle((state) => {
    syncWorkspaceSessionLifecycle(state, ports);
  });

  return () => {
    unsubscribeSelection();
    unsubscribeLifecycle();
  };
}
