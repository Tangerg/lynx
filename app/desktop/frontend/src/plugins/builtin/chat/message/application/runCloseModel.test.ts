import { describe, expect, it } from "vitest";
import type { AgentRunMetrics } from "@/plugins/builtin/agent/public/viewState";
import { runCloseReadout } from "./runCloseModel";

const metrics = (over: Partial<AgentRunMetrics> = {}): AgentRunMetrics => ({
  steps: 0,
  activeDurationMillis: 0,
  usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
  ...over,
});

describe("runCloseReadout", () => {
  // A Run restored across a restart has empty metrics. "0s · 0 steps" ends a turn
  // worse than nothing does.
  it("says nothing rather than nothing-shaped", () => {
    expect(runCloseReadout(null)).toBeNull();
    expect(runCloseReadout(metrics())).toBeNull();
  });

  it("reports only the figures that are real", () => {
    expect(runCloseReadout(metrics({ steps: 12, activeDurationMillis: 246_000 }))).toEqual({
      steps: 12,
      duration: "4m 06s",
    });
  });

  it("carries cost only when it was actually charged", () => {
    const usage = { inputTokens: 82_400, outputTokens: 1200, cacheReadTokens: 0 };
    expect(runCloseReadout(metrics({ usage }))).toEqual({
      inputTokens: "82.4k",
      outputTokens: "1.2k",
    });
    expect(runCloseReadout(metrics({ usage: { ...usage, costUsd: 0 } }))?.cost).toBeUndefined();
    expect(runCloseReadout(metrics({ usage: { ...usage, costUsd: 0.14 } }))?.cost).toBe("$0.14");
  });
});
