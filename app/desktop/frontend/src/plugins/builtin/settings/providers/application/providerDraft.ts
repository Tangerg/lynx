import type { ProviderConfig, SaveProviderInput } from "./providerConfig";

export interface ProviderCredentialsDraft {
  apiKey: string;
  baseUrl: string;
}

export function initialProviderCredentialsDraft(
  provider: Pick<ProviderConfig, "baseUrl">,
): ProviderCredentialsDraft {
  return {
    apiKey: "",
    baseUrl: provider.baseUrl,
  };
}

export function providerCredentialsDirty(
  provider: Pick<ProviderConfig, "baseUrl">,
  draft: ProviderCredentialsDraft,
): boolean {
  return draft.apiKey.trim() !== "" || draft.baseUrl !== provider.baseUrl;
}

export function providerCredentialsInput(
  provider: Pick<ProviderConfig, "id">,
  draft: ProviderCredentialsDraft,
): SaveProviderInput {
  const input: SaveProviderInput = { provider: provider.id };
  const apiKey = draft.apiKey.trim();
  if (apiKey) input.apiKey = apiKey;
  if (draft.baseUrl) input.baseUrl = draft.baseUrl;
  return input;
}
