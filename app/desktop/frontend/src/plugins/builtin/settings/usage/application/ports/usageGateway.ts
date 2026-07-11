import { createSingletonPort } from "@/lib/ports/singletonPort";
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

const port = createSingletonPort<UsageGateway>("Usage gateway is not configured");

export const configureUsageGateway = port.configure;
export const usageGateway = port.get;
