import { describe, expect, it } from "vitest";
import { themeContribution } from "./themeContributions";
import type { ThemePluginSpec } from "./types";

function makeSpec(overrides: Partial<ThemePluginSpec> = {}): ThemePluginSpec {
  return {
    id: "test",
    label: "Test",
    scheme: "dark",
    order: 7,
    brand: { accent: "#1ed760", textOnAccent: "#000000" },
    surfaces: { bg: "#0a0a0a", surface: "#1a1a1a" },
    ink: {
      text: "#eeeeee",
      textBright: "#ffffff",
      textSoft: "#cccccc",
      textMuted: "#999999",
      textFaint: "#666666",
    },
    borders: {
      border: "#2a2a2a",
      borderSoft: "#3a3a3a",
      divider: "#1f1f1f",
    },
    semantic: {
      negative: "#ff5555",
      warning: "#ffaa00",
      info: "#5599ff",
      success: "#55cc55",
    },
    ...overrides,
  };
}

describe("themeContribution", () => {
  it("projects theme plugin specs into theme registry contributions", () => {
    const contribution = themeContribution(makeSpec());

    expect(contribution).toMatchObject({
      id: "test",
      label: "Test",
      scheme: "dark",
      icon: "moon",
      order: 7,
    });
    expect(contribution.tokens?.["color-accent"]).toBe("#1ed760");
    expect(contribution.tokens?.["color-bg"]).toBe("#0a0a0a");
  });

  it("keeps explicit picker icon overrides", () => {
    expect(themeContribution(makeSpec({ icon: "spark", scheme: "light" })).icon).toBe("spark");
  });
});
