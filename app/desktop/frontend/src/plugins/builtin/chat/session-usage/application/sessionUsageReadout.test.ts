import { describe, expect, it } from "vitest";
import { sessionUsageReadout } from "./sessionUsageReadout";

describe("sessionUsageReadout", () => {
  it("hides missing or zero-spend usage", () => {
    expect(sessionUsageReadout(undefined)).toBeNull();
    expect(sessionUsageReadout({})).toBeNull();
    expect(sessionUsageReadout({ inputTokens: 0, outputTokens: 0, costUsd: 0 })).toBeNull();
  });

  it("coalesces absent token counts", () => {
    expect(sessionUsageReadout({ inputTokens: 2400 })).toEqual({
      inputTokens: 2400,
      outputTokens: 0,
    });
  });

  it("keeps cost visible even when token counts are empty", () => {
    expect(sessionUsageReadout({ costUsd: 0.0042 })).toEqual({
      inputTokens: 0,
      outputTokens: 0,
      costUsd: 0.0042,
    });
  });
});
