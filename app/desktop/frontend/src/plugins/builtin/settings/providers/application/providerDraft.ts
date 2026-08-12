import type { ProviderConfiguration } from "./providerConfig";
import type { ProviderUpdate } from "./ports/providerGateway";

export interface ProviderCredentialsDraft {
  apiKey: string;
  baseUrl: string;
}

export function initialProviderCredentialsDraft(
  provider: Pick<ProviderConfiguration, "baseUrl">,
): ProviderCredentialsDraft {
  return {
    apiKey: "",
    baseUrl: provider.baseUrl,
  };
}

export function providerCredentialsDirty(
  provider: Pick<ProviderConfiguration, "baseUrl">,
  draft: ProviderCredentialsDraft,
): boolean {
  return draft.apiKey.trim() !== "" || draft.baseUrl !== provider.baseUrl;
}

export function providerCredentialsValid(
  provider: Pick<ProviderConfiguration, "requiresBaseUrl">,
  draft: ProviderCredentialsDraft,
): boolean {
  return !provider.requiresBaseUrl || draft.baseUrl.trim() !== "";
}

export function providerCredentialsInput(
  provider: Pick<ProviderConfiguration, "id" | "baseUrl">,
  draft: ProviderCredentialsDraft,
): ProviderUpdate {
  const input: ProviderUpdate = { provider: provider.id };
  const apiKey = draft.apiKey.trim();
  if (apiKey) input.apiKey = apiKey;
  if (draft.baseUrl !== provider.baseUrl) {
    const baseUrl = draft.baseUrl.trim();
    input.baseUrl = baseUrl || null;
  }
  return input;
}
