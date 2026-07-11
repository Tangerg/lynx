import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { usageGateway } from "./ports/usageGateway";

export const USAGE_SUMMARY_KEY = "usage.summary";

export interface UsageBreakdownBucket {
  key: string;
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
  runs?: number;
}

export const USAGE_RANGES = [
  { days: 0, label: "usage.range.all" },
  { days: 30, label: "usage.range.30d" },
  { days: 7, label: "usage.range.7d" },
] as const;

export function usageTokens(bucket: { inputTokens?: number; outputTokens?: number }): number {
  return (bucket.inputTokens ?? 0) + (bucket.outputTokens ?? 0);
}

export function useUsageReport(sinceDays: number) {
  return useQuery({
    queryKey: [USAGE_SUMMARY_KEY, sinceDays],
    queryFn: () => usageGateway().loadSummary(sinceDays),
    placeholderData: keepPreviousData,
  });
}
