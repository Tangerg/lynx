import { getContainer } from "@/main/container";
import { describeProblem, rpcErrorText } from "@/lib/rpcErrors";
import type { LyraClient, Provider, ProviderConfigChange } from "@/rpc";
import { installProviderGateway as registerProviderGateway } from "../application/ports/providerGateway";
import type { ProviderGateway } from "../application/ports/providerGateway";
import type { ProviderConfiguration } from "../application/providerModels";
import { ProviderMutationOwner } from "../application/providerMutationOwner";

function runtimeProviderGateway(client: LyraClient): ProviderGateway {
  return {
    async updateProvider(input) {
      const saved = await client.providers.update({
        provider: input.provider,
        apiKey: toWireChange(input.apiKey),
        baseUrl: toWireChange(input.baseUrl),
      });
      return providerConfiguration(saved);
    },
    async setUtilityRole(role) {
      const saved = await client.models.setUtilityRole(role);
      return { provider: saved.provider, model: saved.model };
    },
    async setEmbeddingRole(role) {
      const saved = await client.models.setEmbeddingRole(role);
      return { provider: saved.provider, model: saved.model };
    },
    async testProvider(provider) {
      const result = await client.providers.test(provider);
      return {
        ok: result.ok,
        error: result.ok ? undefined : describeProblem(result.error),
      };
    },
    errorMessage(error) {
      return rpcErrorText(error);
    },
  };
}

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

export interface ProviderGatewayInstallation {
  replaceRuntimeGeneration(): void;
  dispose(): void;
}

export function installProviderGateway(): ProviderGatewayInstallation {
  const gateway = runtimeProviderGateway(getContainer().client());
  const mutationOwner = ProviderMutationOwner.install(gateway);
  const disposeGateway = registerProviderGateway(gateway);
  return {
    replaceRuntimeGeneration: () => mutationOwner.replaceRuntimeGeneration(),
    dispose() {
      mutationOwner.dispose();
      disposeGateway();
    },
  };
}
