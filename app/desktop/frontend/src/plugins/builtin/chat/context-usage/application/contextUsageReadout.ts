/** How full the model's context window is, as the composer reports it. */
export interface ContextUsageReadout {
  /** 0…1. */
  ratio: number;
  /** Whole percent, for the accessible name and the tooltip. */
  percent: number;
  usedTokens: number;
  windowTokens: number;
}

/**
 * The latest model request's total prompt footprint against the served model's
 * context window. Runtime publishes this as `RunProgress.contextTokens`; Session
 * and Run usage totals cannot answer this because they add multiple model rounds.
 *
 * Null whenever the answer would be a guess — no window declared for the model, or
 * no turn has been sent yet. A gauge reading zero says "empty"; that is a claim,
 * and here it would be a false one.
 */
export function contextUsageReadout(
  usedTokens: number | undefined,
  windowTokens: number | undefined,
): ContextUsageReadout | null {
  if (!windowTokens || windowTokens <= 0) return null;
  if (!usedTokens || usedTokens <= 0) return null;
  const used = Math.min(usedTokens, windowTokens);
  const ratio = used / windowTokens;
  return {
    ratio,
    percent: Math.round(ratio * 100),
    usedTokens: used,
    windowTokens,
  };
}
