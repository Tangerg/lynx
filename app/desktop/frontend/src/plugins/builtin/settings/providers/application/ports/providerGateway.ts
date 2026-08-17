import type { ProviderConfiguration, ProviderRole } from "../providerModels";

export interface ProviderUpdate {
  provider: string;
  // undefined preserves, null clears, and a string replaces the setting.
  apiKey?: string | null;
  baseUrl?: string | null;
}

export interface ProviderTestOutcome {
  ok: boolean;
  error?: string;
}

export interface ProviderGateway {
  updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration>;
  setUtilityRole(role: ProviderRole): Promise<ProviderRole>;
  setEmbeddingRole(role: ProviderRole): Promise<ProviderRole>;
  testProvider(provider: string): Promise<ProviderTestOutcome>;
  errorMessage(error: unknown): string | undefined;
}
