import { createSingletonPort } from "@/lib/ports/singletonPort";

export type AgentMemoryDecision = "approve" | "reject";

export interface AgentMemoryAddInput {
  scope: "project" | "user";
  cwd?: string;
  content: string;
}

// AgentMemoryGateway mutates the agent's self-maintained memory review surface:
// approve/reject a pending proposal, edit an item's content, pin/unpin it,
// delete it, or add a user-authored active item. The runtime adapter drives
// agentMemory.* over RPC.
export interface AgentMemoryGateway {
  review(id: string, decision: AgentMemoryDecision): Promise<void>;
  updateContent(id: string, content: string): Promise<void>;
  setPinned(id: string, pinned: boolean): Promise<void>;
  delete(id: string): Promise<void>;
  add(input: AgentMemoryAddInput): Promise<void>;
}

const port = createSingletonPort<AgentMemoryGateway>("Agent memory gateway is not configured");

export const configureAgentMemoryGateway = port.configure;
export const agentMemoryGateway = port.get;
