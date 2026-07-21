import { queryClient } from "@/lib/data/queryClient";
import {
  agentMemoryGateway,
  type AgentMemoryAddInput,
  type AgentMemoryDecision,
} from "./ports/agentMemoryGateway";
import {
  WORKSPACE_AGENT_MEMORY_KEY,
  useAgentMemory as useAgentMemoryQuery,
  type AgentMemoryItemInfo,
  type AgentMemoryQuery,
} from "./workspaceData";

export type { AgentMemoryItemInfo, AgentMemoryQuery };

// Read the review surface for a scope. Disabled (enabled=false) parks the query
// so a not-yet-ready cwd doesn't fire a request; the project scope binds to the
// session's cwd, the user scope ignores it.
export function useAgentMemory(enabled: boolean, scope: AgentMemoryQuery["scope"], cwd?: string) {
  return useAgentMemoryQuery(enabled ? { scope, cwd } : undefined);
}

// Every mutation refetches the list — it's small (one project's active + pending
// items) and there is no server push for agent memory (offline review surface).
async function invalidate(): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
}

export async function reviewAgentMemory(id: string, decision: AgentMemoryDecision): Promise<void> {
  await agentMemoryGateway().review(id, decision);
  await invalidate();
}

export async function updateAgentMemoryContent(id: string, content: string): Promise<void> {
  await agentMemoryGateway().updateContent(id, content);
  await invalidate();
}

export async function setAgentMemoryPinned(id: string, pinned: boolean): Promise<void> {
  await agentMemoryGateway().setPinned(id, pinned);
  await invalidate();
}

export async function deleteAgentMemory(id: string): Promise<void> {
  await agentMemoryGateway().delete(id);
  await invalidate();
}

export async function addAgentMemory(input: AgentMemoryAddInput): Promise<void> {
  await agentMemoryGateway().add(input);
  await invalidate();
}
