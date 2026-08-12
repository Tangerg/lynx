import { getContainer } from "@/main/container";
import { describeProblem, rpcErrorText } from "@/lib/rpcErrors";
import type { Provider, ProviderConfigChange } from "@/rpc";
import { installProviderGateway as registerProviderGateway } from "../application/ports/providerGateway";
import type { ProviderGateway } from "../application/ports/providerGateway";
import type { ProviderConfiguration } from "../application/providerModels";

const gateway: ProviderGateway = {
  async updateProvider(input) {
    const saved = await getContainer()
      .client()
      .providers.update({
        provider: input.provider,
        apiKey: toWireChange(input.apiKey),
        baseUrl: toWireChange(input.baseUrl),
      });
    return providerConfiguration(saved);
  },
  async setUtilityRole(role) {
    const saved = await getContainer().client().models.setUtilityRole(role);
    return { provider: saved.provider, model: saved.model };
  },
  async setEmbeddingRole(role) {
    const saved = await getContainer().client().models.setEmbeddingRole(role);
    return { provider: saved.provider, model: saved.model };
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

function providerConfiguration(provider: Provider): ProviderConfiguration {
  return {
    id: provider.id,
    baseUrl: provider.baseUrl ?? "",
    apiKeyMasked: provider.apiKeyMasked,
    keySource: provider.keySource,
    requiresBaseUrl: provider.requiresBaseUrl,
    embeddingCapable: provider.embeddingCapable,
    defaultEmbeddingModel: provider.defaultEmbeddingModel,
  };
}

function toWireChange(value: string | null | undefined): ProviderConfigChange | undefined {
  if (value === undefined) return undefined;
  return value === null ? { type: "clear" } : { type: "set", value };
}

export function installProviderGateway(): () => void {
  return registerProviderGateway(gateway);
}
