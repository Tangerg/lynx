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
 * The LAST TURN's input count against the window, which is what "how full is the
 * context" means: every turn resends the conversation, so the prompt the most
 * recent request carried IS the occupancy. Session totals cannot answer this —
 * they add every turn together and pass the window long before it is full.
 *
 * `inputTokens` alone, never plus `cacheReadTokens`: the core contract states that
 * cache-read and cache-write are BREAKDOWNS already inside the input total
 * (core/chat/usage.go), so adding them counts the cached prefix twice.
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
  const ratio = Math.min(1, usedTokens / windowTokens);
  return {
    ratio,
    percent: Math.round(ratio * 100),
    usedTokens,
    windowTokens,
  };
}
