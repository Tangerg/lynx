import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface AgentSessionLifecycleSnapshot {
  activeSessionId: string;
  openSessionIds: string[];
}

export interface AgentSessionStatePort {
  useActiveSessionId(): string;
  getActiveSessionId(): string;
  getLifecycleSnapshot(): AgentSessionLifecycleSnapshot;
  subscribeActiveSessionId(onChange: (sessionId: string) => void): () => void;
  subscribeLifecycle(onChange: (snapshot: AgentSessionLifecycleSnapshot) => void): () => void;
  /**
   * Go to a session: hold it open and make it the place the user is. Leaves any
   * promoted view behind — selecting a session means looking at that
   * conversation. One navigation owns the move, so no selection counter is needed.
   */
  selectSession(id: string): void;
  closeSession(id: string): void;
  useDraftSessionIds(): Set<string>;
  /** Is this session a draft — created by a "New" gesture and never used? */
  isDraftSession(id: string): boolean;
  reconcileSessions(liveIds: string[]): void;
  /**
   * Cold start: go to the session the user was last in, if the location doesn't
   * already name one. A no-op when it does — a deeplink or a reload with a
   * session in the URL is a stronger statement about where to be than memory is.
   */
  restoreLastSession(): void;
  markDraftSession(id: string): void;
}

const port = createSingletonPort<AgentSessionStatePort>(
  "Agent session state port is not configured",
);

export const configureAgentSessionStatePort = port.configure;
export const agentSessionState = port.get;
