import { createSingletonPort } from "@/lib/ports/singletonPort";
export interface ProviderUpdate {
  provider: string;
  // undefined preserves, null clears, and a string replaces the setting.
  apiKey?: string | null;
  baseUrl?: string | null;
}

export interface ProviderRole {
  provider?: string;
  model?: string;
}

export interface ProviderTestOutcome {
  ok: boolean;
  error?: string;
}

export interface ProviderGateway {
  updateProvider(input: ProviderUpdate): Promise<void>;
  setUtilityRole(role: ProviderRole): Promise<void>;
  setEmbeddingRole(role: ProviderRole): Promise<void>;
  testProvider(provider: string): Promise<ProviderTestOutcome>;
  errorMessage(error: unknown): string | undefined;
}

const port = createSingletonPort<ProviderGateway>("Provider gateway is not configured");

export const installProviderGateway = port.configure;
export const providerGateway = port.get;
