import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk/types";
import type { AgentInput } from "../../domain/input";

export interface AgentSession {
  /** True when the mounted Session accepted ownership of the input. */
  send: (input: AgentInput, options?: AgentRunStartOptions) => boolean;
  stop: () => void;
}

export interface AgentDefaultSessionPort {
  useDefaultChatSession(): AgentSession;
}

const port = createSingletonPort<AgentDefaultSessionPort>(
  "Agent default session port is not configured",
);

export const configureAgentDefaultSessionPort = port.configure;
export const agentDefaultSession = port.get;
