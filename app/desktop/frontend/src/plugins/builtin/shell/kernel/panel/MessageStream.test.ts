import { describe, expect, it } from "vitest";
import { transcriptTurnContentVisibility } from "./transcriptTurnContentVisibility";

describe("transcript turn content visibility", () => {
  it("keeps historical turns eligible for off-screen rendering skips", () => {
    expect(transcriptTurnContentVisibility(false)).toBe(
      "[content-visibility:auto] [contain-intrinsic-size:auto_220px]",
    );
  });

  it("always renders the tail turn which owns current outcome and HITL controls", () => {
    expect(transcriptTurnContentVisibility(true)).toBeUndefined();
  });
});
