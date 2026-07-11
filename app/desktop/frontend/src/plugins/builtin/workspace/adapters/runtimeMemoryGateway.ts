import { getContainer } from "@/main/container";
import { configureWorkspaceMemoryGateway } from "../application/ports/memoryGateway";
import type { WorkspaceMemoryGateway } from "../application/ports/memoryGateway";

const gateway: WorkspaceMemoryGateway = {
  async save(input) {
    await getContainer().client().memory.update(input);
  },
};

export function installWorkspaceMemoryGateway(): () => void {
  return configureWorkspaceMemoryGateway(gateway);
}
