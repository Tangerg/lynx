import { getContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { configureCodebaseGateway } from "../application/ports/codebaseGateway";
import type { CodebaseGateway } from "../application/ports/codebaseGateway";
import { CodebaseCommandOwner } from "../application/codebaseCommandOwner";

function runtimeCodebaseGateway(client: LyraClient): CodebaseGateway {
  return {
    async search(input) {
      const { cwd, ...query } = input;
      const workspace = await client.workspaces.open(cwd ? { path: cwd } : undefined);
      const result = await workspace.codebase.search(query);
      return result.hits;
    },
    async reindex(cwd) {
      const workspace = await client.workspaces.open(cwd ? { path: cwd } : undefined);
      const operation = await workspace.codebase.reindex();
      return { operationId: operation.operationId };
    },
  };
}

export interface CodebaseGatewayInstallation {
  replaceRuntimeGeneration(): void;
  dispose(): void;
}

export function installCodebaseGateway(): CodebaseGatewayInstallation {
  const gateway = runtimeCodebaseGateway(getContainer().client());
  const commandOwner = CodebaseCommandOwner.install(gateway);
  const disposeGateway = configureCodebaseGateway(gateway);
  return {
    replaceRuntimeGeneration: () => commandOwner.replaceRuntimeGeneration(),
    dispose() {
      commandOwner.dispose();
      disposeGateway();
    },
  };
}
