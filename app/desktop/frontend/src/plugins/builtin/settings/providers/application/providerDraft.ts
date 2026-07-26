import type { ProviderInfo, SaveProviderInput } from "./providerConfig";

export interface ProviderCredentialsDraft {
  apiKey: string;
  baseUrl: string;
}

export function initialProviderCredentialsDraft(
  provider: Pick<ProviderInfo, "baseUrl">,
): ProviderCredentialsDraft {
  return {
    apiKey: "",
    baseUrl: provider.baseUrl,
  };
}

export function providerCredentialsDirty(
  provider: Pick<ProviderInfo, "baseUrl">,
  draft: ProviderCredentialsDraft,
): boolean {
  return draft.apiKey.trim() !== "" || draft.baseUrl !== provider.baseUrl;
}

export function providerCredentialsInput(
  provider: Pick<ProviderInfo, "id">,
  draft: ProviderCredentialsDraft,
): SaveProviderInput {
  const input: SaveProviderInput = { provider: provider.id };
  const apiKey = draft.apiKey.trim();
  if (apiKey) input.apiKey = apiKey;
  if (draft.baseUrl) input.baseUrl = draft.baseUrl;
  return input;
}
