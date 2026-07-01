import { getContainer } from "@/main/container";
import { configureHookTrustGateway } from "../application/ports/hookTrustGateway";
import type { HookTrustGateway } from "../application/ports/hookTrustGateway";

const gateway: HookTrustGateway = {
  async setProjectTrust(projectRoot, trusted) {
    await getContainer().client().workspace.hooks.setTrust(projectRoot, trusted);
  },
};

export function installHookTrustGateway(): void {
  configureHookTrustGateway(gateway);
}
