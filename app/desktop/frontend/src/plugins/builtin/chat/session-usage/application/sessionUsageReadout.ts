export interface SessionUsageSnapshot {
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
}

export interface SessionUsageReadout {
  inputTokens: number;
  outputTokens: number;
  costUsd?: number;
}

export function sessionUsageReadout(
  usage: SessionUsageSnapshot | null | undefined,
): SessionUsageReadout | null {
  if (!usage) return null;

  const inputTokens = usage.inputTokens ?? 0;
  const outputTokens = usage.outputTokens ?? 0;
  const costUsd = usage.costUsd;
  if (inputTokens + outputTokens === 0 && (costUsd ?? 0) === 0) return null;

  return {
    inputTokens,
    outputTokens,
    ...(costUsd !== undefined ? { costUsd } : {}),
  };
}
