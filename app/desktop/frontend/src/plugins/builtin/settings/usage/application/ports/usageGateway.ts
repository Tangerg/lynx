export interface UsageAmount {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
}

export interface UsageBucket extends UsageAmount {
  key: string;
  runs?: number;
}

export interface UsageSummaryReadModel {
  total: UsageAmount;
  byProvider?: UsageBucket[];
  byModel?: UsageBucket[];
  byDay?: UsageBucket[];
  sessions?: number;
  runs?: number;
}

export interface UsageGateway {
  loadSummary(sinceDays: number): Promise<UsageSummaryReadModel>;
}

let port: UsageGateway | null = null;

export function configureUsageGateway(next: UsageGateway): void {
  port = next;
}

export function usageGateway(): UsageGateway {
  if (!port) throw new Error("Usage gateway is not configured");
  return port;
}
