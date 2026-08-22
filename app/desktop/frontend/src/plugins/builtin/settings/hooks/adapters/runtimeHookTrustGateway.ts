import { getContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { HookTrustMutationOwner, type HookTrustGateway } from "../application/hookTrust";

function runtimeHookTrustGateway(client: LyraClient): HookTrustGateway {
  return {
    async setProjectTrust(projectRoot, trusted) {
      await client.hooks.setTrust(projectRoot, trusted);
    },
  };
}

export function installHookTrustGateway() {
  const owner = HookTrustMutationOwner.install(runtimeHookTrustGateway(getContainer().client()));
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose() {
      owner.dispose();
    },
  };
}
