import type { AgentRunStartOptions } from "@/plugins/sdk/types";
import type { AgentInput } from "../../domain/input";

export interface AgentSession {
  send: (input: AgentInput, options?: AgentRunStartOptions) => void;
  stop: () => void;
}

export interface AgentDefaultSessionPort {
  useDefaultChatSession(): AgentSession;
}

let port: AgentDefaultSessionPort | null = null;

export function configureAgentDefaultSessionPort(next: AgentDefaultSessionPort): void {
  port = next;
}

export function agentDefaultSession(): AgentDefaultSessionPort {
  if (!port) throw new Error("Agent default session port is not configured");
  return port;
}
