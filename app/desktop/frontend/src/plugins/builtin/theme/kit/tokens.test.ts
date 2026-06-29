// Unit tests for the buildTokenMap workhorse. Was untested when it lived
// inline in defineThemePlugin.ts — extracted to ./tokens.ts in Batch D
// made it possible. These pin the resolution rules so future theme
// tweaks (e.g. adding a new optional override) can't silently shift
// existing themes' tokens.

import { describe, expect, it } from "vitest";
import type { ThemePluginSpec } from "./types";
import { DARK_SHADOWS, DEFAULT_RADII, LIGHT_SHADOWS, SCHEME_ICON, buildTokenMap } from "./tokens";

function makeSpec(overrides: Partial<ThemePluginSpec> = {}): ThemePluginSpec {
  return {
    id: "test",
    label: "Test",
    scheme: "dark",
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

describe("buildTokenMap", () => {
  it("emits all brand + ink + border + semantic tokens", () => {
    const tokens = buildTokenMap(makeSpec());
    expect(tokens["color-accent"]).toBe("#1ed760");
    expect(tokens["color-text-on-accent"]).toBe("#000000");
    expect(tokens["color-bg"]).toBe("#0a0a0a");
    expect(tokens["color-surface"]).toBe("#1a1a1a");
    expect(tokens["color-text"]).toBe("#eeeeee");
    expect(tokens["color-text-faint"]).toBe("#666666");
    expect(tokens["color-border"]).toBe("#2a2a2a");
    expect(tokens["color-negative"]).toBe("#ff5555");
  });

  it("auto-derives accentBorder + accentPress via colord when not given", () => {
    const tokens = buildTokenMap(makeSpec());
    expect(tokens["color-accent-border"]).not.toBe("#1ed760"); // darkened
    expect(tokens["color-accent-press"]).not.toBe(tokens["color-accent-border"]); // darker still
    expect(tokens["color-accent-border"]).toMatch(/^#[0-9a-f]{6}$/i);
    expect(tokens["color-accent-press"]).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it("respects explicit accentBorder + accentPress overrides", () => {
    const tokens = buildTokenMap(
      makeSpec({
        brand: {
          accent: "#1ed760",
          textOnAccent: "#000",
          accentBorder: "#aabbcc",
          accentPress: "#112233",
        },
      }),
    );
    expect(tokens["color-accent-border"]).toBe("#aabbcc");
    expect(tokens["color-accent-press"]).toBe("#112233");
  });

  it("defaults CTA trio from accent (accent fill + ctaText = textOnAccent)", () => {
    const tokens = buildTokenMap(makeSpec());
    expect(tokens["color-cta"]).toBe("#1ed760");
    expect(tokens["color-cta-text"]).toBe("#000000");
  });

  it("spec.cta overrides CTA per-field", () => {
    const tokens = buildTokenMap(makeSpec({ cta: { cta: "#000000", ctaText: "#ffffff" } }));
    expect(tokens["color-cta"]).toBe("#000000");
    expect(tokens["color-cta-text"]).toBe("#ffffff");
    // Unset field (ctaHover) still falls back to accent-derived value.
    expect(tokens["color-cta-hover"]).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it("emits surface2/3/4 only when explicitly provided", () => {
    const noLadder = buildTokenMap(makeSpec());
    expect(noLadder).not.toHaveProperty("color-surface-2");
    expect(noLadder).not.toHaveProperty("color-surface-3");

    const withLadder = buildTokenMap(
      makeSpec({
        surfaces: { bg: "#0a0a0a", surface: "#1a1a1a", surface2: "#2a", surface3: "#3a" },
      }),
    );
    expect(withLadder["color-surface-2"]).toBe("#2a");
    expect(withLadder["color-surface-3"]).toBe("#3a");
    expect(withLadder).not.toHaveProperty("color-surface-4");
  });

  it("dark scheme picks DARK_SHADOWS; light picks LIGHT_SHADOWS", () => {
    const dark = buildTokenMap(makeSpec({ scheme: "dark" }));
    const light = buildTokenMap(makeSpec({ scheme: "light" }));
    expect(dark["shadow-composer"]).toBe(DARK_SHADOWS.composer);
    expect(light["shadow-composer"]).toBe(LIGHT_SHADOWS.composer);
  });

  it("spec.shadows merges per-key over scheme defaults", () => {
    const tokens = buildTokenMap(makeSpec({ shadows: { composer: "1px 1px red" } }));
    expect(tokens["shadow-composer"]).toBe("1px 1px red");
    // Other shadows still come from DARK defaults.
    expect(tokens["shadow-elevated"]).toBe(DARK_SHADOWS.elevated);
  });

  it("spec.radii merges per-key over DEFAULT_RADII", () => {
    const tokens = buildTokenMap(makeSpec({ radii: { sm: "10px", xl: "20px" } }));
    expect(tokens["radius-sm"]).toBe("10px");
    expect(tokens["radius-xl"]).toBe("20px");
    expect(tokens["radius-md"]).toBe(DEFAULT_RADII.md);
  });

  it("depthStep defaults to 5% but spec can override", () => {
    expect(buildTokenMap(makeSpec())["depth-step"]).toBe("5%");
    expect(buildTokenMap(makeSpec({ depthStep: "8%" }))["depth-step"]).toBe("8%");
  });

  it("extras spread last → wins on key collision", () => {
    const tokens = buildTokenMap(makeSpec({ extras: { "color-accent": "#999999" } }));
    expect(tokens["color-accent"]).toBe("#999999");
  });

  it("explicit ink soft/muted/faint pass through verbatim", () => {
    const tokens = buildTokenMap(makeSpec());
    expect(tokens["color-text-soft"]).toBe("#cccccc");
    expect(tokens["color-text-muted"]).toBe("#999999");
    expect(tokens["color-text-faint"]).toBe("#666666");
  });

  it("omitted ink soft/muted/faint derive as text at decreasing alpha", () => {
    const tokens = buildTokenMap(makeSpec({ ink: { text: "#eeeeee", textBright: "#ffffff" } }));
    expect(tokens["color-text"]).toBe("#eeeeee");
    expect(tokens["color-text-soft"]).toBe(
      "color-mix(in oklab, var(--color-text) 82%, transparent)",
    );
    expect(tokens["color-text-muted"]).toBe(
      "color-mix(in oklab, var(--color-text) 56%, transparent)",
    );
    expect(tokens["color-text-faint"]).toBe(
      "color-mix(in oklab, var(--color-text) 38%, transparent)",
    );
  });

  it("SCHEME_ICON maps dark/light to moon/sun", () => {
    expect(SCHEME_ICON.dark).toBe("moon");
    expect(SCHEME_ICON.light).toBe("sun");
  });
});
