import { getContainer } from "@/main/container";
import { describeProblem, rpcErrorText } from "@/lib/rpcErrors";
import type { ProviderConfigChange } from "@/rpc";
import { installProviderGateway as registerProviderGateway } from "../application/ports/providerGateway";
import type { ProviderGateway } from "../application/ports/providerGateway";

const gateway: ProviderGateway = {
  async updateProvider(input) {
    await getContainer()
      .client()
      .providers.update({
        provider: input.provider,
        apiKey: toWireChange(input.apiKey),
        baseUrl: toWireChange(input.baseUrl),
      });
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

function toWireChange(value: string | null | undefined): ProviderConfigChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

export function installProviderGateway(): () => void {
  return registerProviderGateway(gateway);
}
