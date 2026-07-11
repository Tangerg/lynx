import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";

export interface PendingAgentMessage {
  input: AgentInput;
  runOptions: AgentRunStartOptions;
}

export interface AgentSessionLifecycleSnapshot {
  activeSessionId: string;
  openSessionIds: string[];
}

export interface AgentSessionSelectionSnapshot {
  activeSessionId: string;
  selectionEpoch: number;
}

export interface AgentSessionStatePort {
  useActiveSessionId(): string;
  getActiveSessionId(): string;
  getLifecycleSnapshot(): AgentSessionLifecycleSnapshot;
  subscribeActiveSessionId(onChange: (sessionId: string) => void): () => void;
  subscribeLifecycle(onChange: (snapshot: AgentSessionLifecycleSnapshot) => void): () => void;
  subscribeSelection(
    onChange: (
      snapshot: AgentSessionSelectionSnapshot,
      previous: AgentSessionSelectionSnapshot,
    ) => void,
  ): () => void;
  selectSession(id: string): void;
  closeSession(id: string): void;
  useDraftSessionIds(): Set<string>;
  useSelectSession(): (id: string) => void;
  reconcileSessions(liveIds: string[]): void;
  markDraftSession(id: string): void;
  graduateDraftSession(id: string): void;
  setPendingMessage(id: string, message: PendingAgentMessage): void;
  takePendingMessage(id: string): PendingAgentMessage | undefined;
}

const port = createSingletonPort<AgentSessionStatePort>(
  "Agent session state port is not configured",
);

export const configureAgentSessionStatePort = port.configure;
export const agentSessionState = port.get;
