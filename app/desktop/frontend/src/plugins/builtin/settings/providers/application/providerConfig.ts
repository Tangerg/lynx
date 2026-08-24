import { useCallback, useSyncExternalStore } from "react";
import { t } from "@/lib/i18n";
import {
  type ProviderConfiguration,
  providerRoleIsAvailable,
  useEmbeddingRole,
  useModels,
  useProviders,
  useUtilityRole,
} from "./providerQueries";
import type { ProviderRole } from "./providerModels";
import type { ProviderUpdate } from "./ports/providerGateway";
import { ProviderMutationOwner, providerMutationWasRetired } from "./providerMutationOwner";

// Provider configuration mutations (providers.update / providers.test).
// Counterpart to the read-side useProviders() query.

export type { ProviderConfiguration };

export function useProviderConfigs() {
  return useProviders();
}

export function useProviderMutationMaterialGeneration(): number {
  return useSyncExternalStore(
    ProviderMutationOwner.subscribeMaterialGeneration,
    ProviderMutationOwner.materialGeneration,
    ProviderMutationOwner.materialGeneration,
  );
}

export function useProviderRoleConfig() {
  const utilityRole = useUtilityRole();
  const embeddingRole = useEmbeddingRole();
  const models = useModels();
  const providers = useProviders();
  return { utilityRole, embeddingRole, models, providers };
}

export function useUtilityModelConfig() {
  const { utilityRole, models, providers } = useProviderRoleConfig();
  const role = utilityRole.data;
  const modelOptions = models.data ?? [];
  const providerConfigs = providers.data ?? [];
  const selected =
    role?.provider && role.model
      ? (modelOptions.find(
          (model) => model.provider === role.provider && model.id === role.model,
        ) ?? null)
      : null;
  return {
    role,
    modelOptions,
    selected,
    isSet: Boolean(role?.model),
    isAvailable: providerRoleIsAvailable(role, providerConfigs),
    isError: models.isError,
  };
}

export function useEmbeddingModelConfig() {
  const { embeddingRole, providers } = useProviderRoleConfig();
  const role = embeddingRole.data;
  const providerConfigs = providers.data ?? [];
  return {
    role,
    providers: providerConfigs,
    capableProviders: providerConfigs.filter(
      (provider) => provider.embeddingCapable && provider.apiKeyMasked !== "",
    ),
    isSet: Boolean(role?.model),
    isAvailable: providerRoleIsAvailable(role, providerConfigs),
  };
}

export { providerMutationWasRetired };

export async function updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration> {
  return ProviderMutationOwner.current().updateProvider(input);
}

export function useUpdateProvider(): (input: ProviderUpdate) => Promise<ProviderConfiguration> {
  return useCallback((input) => {
    return updateProvider(input);
  }, []);
}

/**
 * Point the maintenance work (compaction / extraction / titling) at a
 * (provider, model) — an empty model clears it back to the main turn model
 * (models.setUtilityRole). The runtime validates by resolving the client, so
 * an unconfigured provider / unknown model fails server-side; we flatten that
 * to `{ ok:false, error }` here (mirroring useTestProvider) so the pane —
 * which must not import @/rpc — renders the reason inline. On success the
 * utility-role query is refetched so the pane reflects the stored value.
 */
export async function setUtilityRole(role: ProviderRole): Promise<TestOutcome> {
  const owner = ProviderMutationOwner.current();
  try {
    await owner.setUtilityRole(role);
    return { ok: true };
  } catch (error) {
    if (providerMutationWasRetired(error)) throw error;
    const detail = owner.errorMessage(error);
    return {
      ok: false,
      error: detail ?? (error instanceof Error ? error.message : t("providers.utility.error")),
    };
  }
}

/**
 * Select the optional embedding model for agent-memory ranking. An empty model
 * leaves memory search keyword-only (models.setEmbeddingRole).
 * Validated server-side (the provider must be embedding-capable + configured);
 * flattened to `{ ok, error }` so the pane renders the reason inline. Refetches
 * the embedding-role query on success.
 */
export async function setEmbeddingRole(role: ProviderRole): Promise<TestOutcome> {
  const owner = ProviderMutationOwner.current();
  try {
    await owner.setEmbeddingRole(role);
    return { ok: true };
  } catch (error) {
    if (providerMutationWasRetired(error)) throw error;
    const detail = owner.errorMessage(error);
    return {
      ok: false,
      error: detail ?? (error instanceof Error ? error.message : t("providers.embedding.error")),
    };
  }
}

export interface TestOutcome {
  ok: boolean;
  /** Human-readable failure reason (e.g. a 401 detail), already flattened. */
  error?: string;
}

/**
 * Live-probe a provider (providers.test): the runtime sends a minimal request
 * with the provider's key. A failed probe comes back as `{ ok:false, error }`
 * (NOT an RPC error), so callers render the reason inline.
 */
export function useTestProvider(): (provider: string) => Promise<TestOutcome> {
  return useCallback(async (provider) => {
    const res = await ProviderMutationOwner.current().testProvider(provider);
    return {
      ok: res.ok,
      error: res.ok ? undefined : (res.error ?? t("providers.error.test")),
    };
  }, []);
}
