import { getContainer } from "@/main/container";
import type { AgentMemoryGateway } from "../application/ports/agentMemoryGateway";
import type { AgentMemoryItem } from "@/rpc";
import type { AgentMemoryEntry } from "../application/workspaceQueries";
import { AgentMemoryMutationOwner } from "../application/agentMemoryMutationOwner";

function memoryEntry(item: AgentMemoryItem): AgentMemoryEntry {
  return {
    id: item.id,
    scope: item.scope,
    content: item.content,
    origin: item.origin,
    status: item.status,
    pinned: item.pinned,
    sessionId: item.sessionId ?? "",
    day: item.day ?? "",
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

const gateway: AgentMemoryGateway = {
  async review(id, decision) {
    await getContainer().client().agentMemory.review(id, decision);
  },
  async updateContent(id, content) {
    return memoryEntry(await getContainer().client().agentMemory.update({ id, content }));
  },
  async setPinned(id, pinned) {
    return memoryEntry(await getContainer().client().agentMemory.update({ id, pinned }));
  },
  async delete(id) {
    await getContainer().client().agentMemory.delete(id);
  },
  async add(input) {
    const client = getContainer().client();
    if (input.scope === "user") {
      return memoryEntry(await client.agentMemory.add({ scope: "user", content: input.content }));
    }
    const workspace = await client.workspaces.open(input.cwd ? { path: input.cwd } : undefined);
    return memoryEntry(await workspace.agentMemory.add(input.content));
  },
};

export function installAgentMemoryGateway() {
  const mutationOwner = AgentMemoryMutationOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () => mutationOwner.replaceRuntimeGeneration(),
    dispose: () => mutationOwner.dispose(),
  };
}
