import { describe, expect, it } from "vitest";
import { contextUsageReadout } from "./contextUsageReadout";

describe("contextUsageReadout", () => {
  it("matches Codex by clamping both the ring and displayed usage to the model window", () => {
    expect(contextUsageReadout(300_000, 258_000)).toEqual({
      ratio: 1,
      percent: 100,
      usedTokens: 258_000,
      windowTokens: 258_000,
    });
  });

  it("does not draw a context claim without both Runtime facts", () => {
    expect(contextUsageReadout(undefined, 258_000)).toBeNull();
    expect(contextUsageReadout(198_000, undefined)).toBeNull();
  });
});
