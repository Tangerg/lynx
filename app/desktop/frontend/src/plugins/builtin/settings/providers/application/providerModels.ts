export interface ProviderRole {
  provider?: string;
  model?: string;
}

export interface ProviderConfiguration {
  id: string;
  baseUrl: string;
  apiKeyMasked: string;
  keySource?: "stored" | "env";
  requiresBaseUrl?: boolean;
  embeddingCapable?: boolean;
  defaultEmbeddingModel?: string;
}
