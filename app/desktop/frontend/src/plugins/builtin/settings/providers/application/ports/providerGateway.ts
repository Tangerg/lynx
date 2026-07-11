import { createSingletonPort } from "@/lib/ports/singletonPort";
export interface ProviderCredentials {
  provider: string;
  apiKey?: string;
  baseUrl?: string;
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
  configureProvider(input: ProviderCredentials): Promise<void>;
  setUtilityRole(role: ProviderRole): Promise<void>;
  setEmbeddingRole(role: ProviderRole): Promise<void>;
  testProvider(provider: string): Promise<ProviderTestOutcome>;
  errorMessage(error: unknown): string | undefined;
}

const port = createSingletonPort<ProviderGateway>("Provider gateway is not configured");

export const configureProviderGateway = port.configure;
export const providerGateway = port.get;
