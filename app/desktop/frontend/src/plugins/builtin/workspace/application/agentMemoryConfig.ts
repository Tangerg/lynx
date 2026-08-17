import { type AgentMemoryAddInput, type AgentMemoryDecision } from "./ports/agentMemoryGateway";
import {
  useAgentMemory as useAgentMemoryQuery,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "./workspaceQueries";
import {
  AgentMemoryMutationOwner,
  agentMemoryMutationWasRetired,
  agentMemoryQuery,
} from "./agentMemoryMutationOwner";

export type { AgentMemoryEntry, AgentMemoryQuery };
export { agentMemoryMutationWasRetired, agentMemoryQuery };

// Read the review surface for a scope. Disabled (enabled=false) parks the query
// so a not-yet-ready cwd doesn't fire a request; the project scope binds to the
// session's cwd, the user scope ignores it.
export function useAgentMemory(enabled: boolean, scope: AgentMemoryQuery["scope"], cwd?: string) {
  return useAgentMemoryQuery(enabled ? agentMemoryQuery(scope, cwd) : undefined);
}

export async function reviewAgentMemory(id: string, decision: AgentMemoryDecision): Promise<void> {
  await AgentMemoryMutationOwner.current().review(id, decision);
}

export async function updateAgentMemoryContent(id: string, content: string): Promise<void> {
  await AgentMemoryMutationOwner.current().updateContent(id, content);
}

export async function setAgentMemoryPinned(id: string, pinned: boolean): Promise<void> {
  await AgentMemoryMutationOwner.current().setPinned(id, pinned);
}

export async function deleteAgentMemory(id: string): Promise<void> {
  await AgentMemoryMutationOwner.current().delete(id);
}

export async function addAgentMemory(input: AgentMemoryAddInput): Promise<AgentMemoryEntry> {
  return AgentMemoryMutationOwner.current().add(input);
}
