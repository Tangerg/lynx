import { createDataQuery } from "@/plugins/sdk";
import type { ProviderConfiguration, ProviderRole } from "./providerModels";
import { SelectableModel } from "./selectableModel";

export type { ProviderConfiguration, ProviderRole } from "./providerModels";
export { SelectableModel } from "./selectableModel";

/**
 * A stored role is configuration intent, not proof that its provider is usable
 * right now. Stored credentials can be cleared (or an environment credential
 * can disappear between launches) without erasing the role. Join the role with
 * providers.list whenever a feature needs the effective availability.
 */
export function providerRoleIsAvailable(
  role: ProviderRole | undefined,
  providers: readonly ProviderConfiguration[],
): boolean {
  return Boolean(
    role?.provider &&
    role.model &&
    providers.some((provider) => provider.id === role.provider && provider.apiKeyMasked !== ""),
  );
}

export const PROVIDERS_KEY = "providers";
export const MODELS_KEY = "models";
export const UTILITY_ROLE_KEY = "utility-role";
export const EMBEDDING_ROLE_KEY = "embedding-role";
export const useModels = createDataQuery<SelectableModel[]>(MODELS_KEY);
export const useProviders = createDataQuery<ProviderConfiguration[]>(PROVIDERS_KEY);
export const useUtilityRole = createDataQuery<ProviderRole>(UTILITY_ROLE_KEY);
export const useEmbeddingRole = createDataQuery<ProviderRole>(EMBEDDING_ROLE_KEY);
