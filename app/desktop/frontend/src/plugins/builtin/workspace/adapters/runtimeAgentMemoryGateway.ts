import { getContainer } from "@/main/container";
import { configureAgentMemoryGateway } from "../application/ports/agentMemoryGateway";
import type { AgentMemoryGateway } from "../application/ports/agentMemoryGateway";

const gateway: AgentMemoryGateway = {
  async review(id, decision) {
    await getContainer().client().agentMemory.review(id, decision);
  },
  async updateContent(id, content) {
    await getContainer().client().agentMemory.update({ id, content });
  },
  async setPinned(id, pinned) {
    await getContainer().client().agentMemory.update({ id, pinned });
  },
  async delete(id) {
    await getContainer().client().agentMemory.delete(id);
  },
  async add(input) {
    const client = getContainer().client();
    if (input.scope === "user") {
      await client.agentMemory.add({ scope: "user", content: input.content });
      return;
    }
    const workspace = await client.workspaces.open(input.cwd ? { path: input.cwd } : undefined);
    await workspace.agentMemory.add(input.content);
  },
};

export function installAgentMemoryGateway(): () => void {
  return configureAgentMemoryGateway(gateway);
}
