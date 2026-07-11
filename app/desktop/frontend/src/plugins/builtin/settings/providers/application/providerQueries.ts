import { createDataQuery, createParameterizedDataQuery } from "@/lib/data/dataQuery";

export interface ProviderRoleSelection {
  provider?: string;
  model?: string;
}

export interface CodebaseStatusReadModel {
  state: "none" | "indexing" | "ready" | "error";
  modelId?: string;
  fileCount: number;
  chunkCount: number;
  indexedAt?: string;
  truncated?: boolean;
  error?: string;
}

export interface SelectableModel {
  id: string;
  provider: string;
  label: string;
  multimodal: boolean;
  contextWindow?: number;
}

export interface ProviderInfo {
  id: string;
  baseUrl: string;
  apiKeyMasked: string;
  keySource?: "stored" | "env";
  embeddingCapable?: boolean;
  defaultEmbeddingModel?: string;
}

export interface CodebaseStatusQuery {
  cwd?: string;
}

export const PROVIDERS_KEY = "providers";
export const MODELS_KEY = "models";
export const UTILITY_ROLE_KEY = "utility-role";
export const EMBEDDING_ROLE_KEY = "embedding-role";
export const CODEBASE_STATUS_KEY = "codebase-status";

export const useModels = createDataQuery<SelectableModel[]>(MODELS_KEY);
export const useProviders = createDataQuery<ProviderInfo[]>(PROVIDERS_KEY);
export const useUtilityRole = createDataQuery<ProviderRoleSelection>(UTILITY_ROLE_KEY);
export const useEmbeddingRole = createDataQuery<ProviderRoleSelection>(EMBEDDING_ROLE_KEY);
export const useCodebaseStatus = createParameterizedDataQuery<
  CodebaseStatusQuery,
  CodebaseStatusReadModel
>(CODEBASE_STATUS_KEY);
