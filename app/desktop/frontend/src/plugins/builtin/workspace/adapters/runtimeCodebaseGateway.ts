import { getContainer } from "@/main/container";
import { configureCodebaseGateway } from "../application/ports/codebaseGateway";
import type { CodebaseGateway } from "../application/ports/codebaseGateway";

const gateway: CodebaseGateway = {
  async search(input) {
    const result = await getContainer().client().codebase.search(input);
    return result.hits;
  },
  async reindex(cwd) {
    await getContainer().client().codebase.reindex(cwd);
  },
};

export function installCodebaseGateway(): void {
  configureCodebaseGateway(gateway);
}
