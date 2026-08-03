import { describe, expect, it } from "vitest";
import {
  UI_FONT_SIZE_DEFAULT_PX,
  UI_FONT_SIZE_MAX_PX,
  UI_FONT_SIZE_MIN_PX,
  normalizeUiFontSize,
  uiTypeLadder,
  uiTypeLadderCssVariables,
} from "./typography";

describe("normalizeUiFontSize", () => {
  it("falls back to the default for absent or non-finite input", () => {
    expect(normalizeUiFontSize(null)).toBe(UI_FONT_SIZE_DEFAULT_PX);
    expect(normalizeUiFontSize(undefined)).toBe(UI_FONT_SIZE_DEFAULT_PX);
    expect(normalizeUiFontSize(Number.NaN)).toBe(UI_FONT_SIZE_DEFAULT_PX);
  });

  it("clamps and rounds into the supported range", () => {
    expect(normalizeUiFontSize(2)).toBe(UI_FONT_SIZE_MIN_PX);
    expect(normalizeUiFontSize(99)).toBe(UI_FONT_SIZE_MAX_PX);
    expect(normalizeUiFontSize(12.6)).toBe(13);
  });
});

describe("uiTypeLadder", () => {
  it("lands the default base on the whole-pixel grid", () => {
    expect(uiTypeLadder(UI_FONT_SIZE_DEFAULT_PX)).toEqual({
      "ui-2xs": 11,
      "ui-xs": 12,
      "ui-sm": 13,
      "ui-md": 14,
      prose: 16,
      code: 13,
    });
  });

  it("never inverts a step across the whole base range", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const ladder = uiTypeLadder(base);
      const ascending = [ladder["ui-2xs"], ladder["ui-xs"], ladder["ui-sm"], ladder["ui-md"]];
      expect(ascending).toEqual([...ascending].sort((a, b) => a - b));
      expect(ladder.code).toBeLessThanOrEqual(ladder["ui-md"]);
    }
  });

  // Reading text may never end up at or below the chrome around it — that
  // collapse is exactly what the step was added to prevent, and rounding plus the
  // ceiling are where it could happen quietly.
  it("keeps reading text above the chrome across the whole base range", () => {
    for (let base = UI_FONT_SIZE_MIN_PX; base <= UI_FONT_SIZE_MAX_PX; base += 1) {
      const ladder = uiTypeLadder(base);
      expect(ladder.prose).toBeGreaterThan(ladder["ui-md"]);
    }
  });

  it("keeps the small end legible when the base shrinks", () => {
    // Ratio alone would put ui-2xs at 8px here; the floor holds it at 9.
    expect(uiTypeLadder(UI_FONT_SIZE_MIN_PX)["ui-2xs"]).toBe(9);
  });

  it("normalizes the base before deriving", () => {
    expect(uiTypeLadder(null)).toEqual(uiTypeLadder(UI_FONT_SIZE_DEFAULT_PX));
    expect(uiTypeLadder(1000)).toEqual(uiTypeLadder(UI_FONT_SIZE_MAX_PX));
  });
});

describe("uiTypeLadderCssVariables", () => {
  it("emits every ladder step as a px custom property", () => {
    expect(uiTypeLadderCssVariables(UI_FONT_SIZE_DEFAULT_PX)).toEqual({
      "--fs-ui-2xs": "11px",
      "--fs-ui-xs": "12px",
      "--fs-ui-sm": "13px",
      "--fs-ui-md": "14px",
      "--fs-prose": "16px",
      "--fs-code": "13px",
    });
  });
});
