import { getContainer } from "@/main/container";
import { configureWorkspaceMemoryGateway } from "../application/ports/memoryGateway";
import type { WorkspaceMemoryGateway } from "../application/ports/memoryGateway";

const gateway: WorkspaceMemoryGateway = {
  async save(input) {
    const { cwd, ...update } = input;
    const workspace = await getContainer()
      .client()
      .workspaces.open(cwd ? { path: cwd } : undefined);
    await workspace.memory.update(update);
  },
};

export function installWorkspaceMemoryGateway(): () => void {
  return configureWorkspaceMemoryGateway(gateway);
}
