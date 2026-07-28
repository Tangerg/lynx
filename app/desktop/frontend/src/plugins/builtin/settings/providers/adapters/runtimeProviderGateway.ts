import { getContainer } from "@/main/container";
import { describeProblem, rpcErrorText } from "@/lib/rpcErrors";
import { configureProviderGateway } from "../application/ports/providerGateway";
import type { ProviderGateway } from "../application/ports/providerGateway";

const gateway: ProviderGateway = {
  async configureProvider(input) {
    await getContainer().client().providers.configure(input);
  },
  async setUtilityRole(role) {
    await getContainer().client().models.setUtilityRole(role);
  },
  async setEmbeddingRole(role) {
    await getContainer().client().models.setEmbeddingRole(role);
  },
  async testProvider(provider) {
    const result = await getContainer().client().providers.test(provider);
    return {
      ok: result.ok,
      error: result.ok ? undefined : describeProblem(result.error),
    };
  },
  errorMessage(error) {
    return rpcErrorText(error);
  },
};

export function installProviderGateway(): () => void {
  return configureProviderGateway(gateway);
}
