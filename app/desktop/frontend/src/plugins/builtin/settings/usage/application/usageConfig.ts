import type { UsageBucket } from "@/rpc";
import { useUsageSummary } from "@/lib/data/useUsage";

export type UsageBreakdownBucket = UsageBucket;

export const USAGE_RANGES = [
  { days: 0, label: "usage.range.all" },
  { days: 30, label: "usage.range.30d" },
  { days: 7, label: "usage.range.7d" },
] as const;

export function usageTokens(bucket: { inputTokens?: number; outputTokens?: number }): number {
  return (bucket.inputTokens ?? 0) + (bucket.outputTokens ?? 0);
}

export function useUsageReport(sinceDays: number) {
  return useUsageSummary(sinceDays);
}
