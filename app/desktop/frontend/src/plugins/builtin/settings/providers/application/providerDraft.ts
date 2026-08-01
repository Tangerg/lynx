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

export function providerCredentialsInput(
  provider: Pick<ProviderConfiguration, "id" | "baseUrl">,
  draft: ProviderCredentialsDraft,
): ProviderUpdate {
  const input: ProviderUpdate = { provider: provider.id };
  const apiKey = draft.apiKey.trim();
  if (apiKey) input.apiKey = apiKey;
  if (draft.baseUrl !== provider.baseUrl) {
    input.baseUrl = draft.baseUrl || null;
  }
  return input;
}
