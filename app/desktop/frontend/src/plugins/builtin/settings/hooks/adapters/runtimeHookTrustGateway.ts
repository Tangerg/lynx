import { getContainer } from "@/main/container";
import { configureHookTrustGateway } from "../application/ports/hookTrustGateway";
import type { HookTrustGateway } from "../application/ports/hookTrustGateway";

const gateway: HookTrustGateway = {
  async setProjectTrust(projectRoot, trusted) {
    await getContainer().client().hooks.setTrust(projectRoot, trusted);
  },
};

export function installHookTrustGateway(): () => void {
  return configureHookTrustGateway(gateway);
}
