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

let port: ProviderGateway | null = null;

export function configureProviderGateway(next: ProviderGateway): void {
  port = next;
}

export function providerGateway(): ProviderGateway {
  if (!port) throw new Error("Provider gateway is not configured");
  return port;
}
