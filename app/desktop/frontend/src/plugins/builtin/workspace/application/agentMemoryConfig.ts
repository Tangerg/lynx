import { queryClient } from "@/lib/queryClient";
import { createSerialTaskQueue } from "@/lib/serialTaskQueue";
import {
  agentMemoryGateway,
  type AgentMemoryAddInput,
  type AgentMemoryDecision,
} from "./ports/agentMemoryGateway";
import {
  WORKSPACE_AGENT_MEMORY_KEY,
  useAgentMemory as useAgentMemoryQuery,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "./workspaceQueries";

export type { AgentMemoryEntry, AgentMemoryQuery };

export function agentMemoryQuery(scope: AgentMemoryQuery["scope"], cwd?: string): AgentMemoryQuery {
  return scope === "user" ? { scope } : { scope, cwd };
}

// Read the review surface for a scope. Disabled (enabled=false) parks the query
// so a not-yet-ready cwd doesn't fire a request; the project scope binds to the
// session's cwd, the user scope ignores it.
export function useAgentMemory(enabled: boolean, scope: AgentMemoryQuery["scope"], cwd?: string) {
  return useAgentMemoryQuery(enabled ? agentMemoryQuery(scope, cwd) : undefined);
}

// Every mutation refetches the list — it's small (one project's active + pending
// items) and there is no server push for agent memory (offline review surface).
async function invalidate(): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_AGENT_MEMORY_KEY] });
}

const memoryChanges = createSerialTaskQueue();

function commitAgentMemoryItem(saved: AgentMemoryEntry): void {
  queryClient.setQueriesData<AgentMemoryEntry[]>(
    { queryKey: [WORKSPACE_AGENT_MEMORY_KEY] },
    (current) => {
      if (!current) return current;
      const index = current.findIndex((item) => item.id === saved.id);
      if (index < 0) return current;
      return current.map((item) => (item.id === saved.id ? saved : item));
    },
  );
}

function commitAddedAgentMemory(input: AgentMemoryAddInput, saved: AgentMemoryEntry): void {
  const query = agentMemoryQuery(input.scope, input.cwd);
  queryClient.setQueryData<AgentMemoryEntry[]>([WORKSPACE_AGENT_MEMORY_KEY, query], (current) =>
    current ? [saved, ...current] : current,
  );
}

export async function reviewAgentMemory(id: string, decision: AgentMemoryDecision): Promise<void> {
  await memoryChanges.run(async () => {
    await agentMemoryGateway().review(id, decision);
    await invalidate();
  });
}

export async function updateAgentMemoryContent(id: string, content: string): Promise<void> {
  await memoryChanges.run(async () => {
    commitAgentMemoryItem(await agentMemoryGateway().updateContent(id, content));
    await invalidate();
  });
}

export async function setAgentMemoryPinned(id: string, pinned: boolean): Promise<void> {
  await memoryChanges.run(async () => {
    commitAgentMemoryItem(await agentMemoryGateway().setPinned(id, pinned));
    await invalidate();
  });
}

export async function deleteAgentMemory(id: string): Promise<void> {
  await memoryChanges.run(async () => {
    await agentMemoryGateway().delete(id);
    queryClient.setQueriesData<AgentMemoryEntry[]>(
      { queryKey: [WORKSPACE_AGENT_MEMORY_KEY] },
      (current) => current?.filter((item) => item.id !== id),
    );
    await invalidate();
  });
}

export async function addAgentMemory(input: AgentMemoryAddInput): Promise<AgentMemoryEntry> {
  return memoryChanges.run(async () => {
    const saved = await agentMemoryGateway().add(input);
    commitAddedAgentMemory(input, saved);
    await invalidate();
    return saved;
  });
}
