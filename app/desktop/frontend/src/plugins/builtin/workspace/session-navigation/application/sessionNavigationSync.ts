import type { AgentSessionLifecycleSnapshot } from "@/plugins/builtin/agent/public/session";

export type AgentSessionListener = (sessionId: string) => void;
export type AgentSessionLifecycleListener = (state: AgentSessionLifecycleSnapshot) => void;

export interface WorkspaceSessionNavigationPorts {
  activeSessionId: () => string;
  lifecycleSnapshot: () => AgentSessionLifecycleSnapshot;
  subscribeActiveSessionId: (listener: AgentSessionListener) => () => void;
  subscribeLifecycle: (listener: AgentSessionLifecycleListener) => () => void;
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

export function syncWorkspaceSessionLifecycle(
  state: AgentSessionLifecycleSnapshot,
  ports: Pick<WorkspaceSessionNavigationPorts, "forgetSessionScopes">,
): void {
  ports.forgetSessionScopes(state.openSessionIds);
}

/**
 * Keep the dock's per-session memory pointed at the session the user is in.
 *
 * There used to be a second rule here — "any re-selection returns to the chat" —
 * driven by a counter the session store bumped on every select. Going to a
 * session is now one navigation that clears the promoted view itself, so the
 * counter, the snapshot type that carried it, and this branch are all gone.
 */
export function bindWorkspaceSessionNavigation(ports: WorkspaceSessionNavigationPorts): () => void {
  ports.activateSessionScope(ports.activeSessionId());
  ports.forgetSessionScopes(ports.lifecycleSnapshot().openSessionIds);

  const unsubscribeSession = ports.subscribeActiveSessionId((sessionId) => {
    ports.activateSessionScope(sessionId);
  });
  const unsubscribeLifecycle = ports.subscribeLifecycle((state) => {
    syncWorkspaceSessionLifecycle(state, ports);
  });

  return () => {
    unsubscribeSession();
    unsubscribeLifecycle();
  };
}
