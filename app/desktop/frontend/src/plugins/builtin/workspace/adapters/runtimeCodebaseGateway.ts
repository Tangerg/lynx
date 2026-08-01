import { getContainer } from "@/main/container";
import { configureCodebaseGateway } from "../application/ports/codebaseGateway";
import type { CodebaseGateway } from "../application/ports/codebaseGateway";

const gateway: CodebaseGateway = {
  async search(input) {
    const { cwd, ...query } = input;
    const workspace = await getContainer()
      .client()
      .workspaces.open(cwd ? { path: cwd } : undefined);
    const result = await workspace.codebase.search(query);
    return result.hits;
  },
  async reindex(cwd) {
    const workspace = await getContainer()
      .client()
      .workspaces.open(cwd ? { path: cwd } : undefined);
    await workspace.codebase.reindex();
  },
};

export function installCodebaseGateway(): () => void {
  return configureCodebaseGateway(gateway);
}
