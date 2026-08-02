// Unit tests for the buildTokenMap workhorse. Was untested when it lived
// inline in defineColorThemePlugin.ts — extracted to ./tokens.ts in Batch D
// made it possible. These pin the resolution rules so future theme
// tweaks (e.g. adding a new optional override) can't silently shift
// existing themes' tokens.

import { describe, expect, it } from "vitest";
import type { ColorThemePluginSpec } from "./types";
import { SCHEME_ICON, buildTokenMap } from "./tokens";

function makeSpec(overrides: Partial<ColorThemePluginSpec> = {}): ColorThemePluginSpec {
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

  // The -2/-3/-4 steps are the color-mix() ladder in globals.css so they track
  // --depth-step (the contrast preference). A theme must not be able to pin
  // them, or the contrast slider goes partially dead on that theme.
  it("never emits the derived surface ladder steps", () => {
    const tokens = buildTokenMap(makeSpec({ surfaces: { bg: "#0a0a0a", surface: "#1a1a1a" } }));
    expect(tokens["color-surface"]).toBe("#1a1a1a");
    expect(tokens).not.toHaveProperty("color-surface-2");
    expect(tokens).not.toHaveProperty("color-surface-3");
    expect(tokens).not.toHaveProperty("color-surface-4");
  });

  it("never emits visual-style tokens", () => {
    const tokens = buildTokenMap(makeSpec());
    expect(Object.keys(tokens).filter((key) => key.startsWith("style-shape-"))).toEqual([]);
    expect(Object.keys(tokens).filter((key) => key.startsWith("shadow-"))).toEqual([]);
  });

  it("never emits the ladder step — the contrast preference owns it", () => {
    expect(buildTokenMap(makeSpec())).not.toHaveProperty("depth-step");
  });

  it("elevated falls back to the first ladder rung, sunken to a per-scheme neutral", () => {
    const dark = buildTokenMap(makeSpec({ scheme: "dark" }));
    expect(dark["color-elevated"]).toBe("var(--color-surface-2)");
    expect(dark["color-sunken"]).toBe("#1c1c21");
    const light = buildTokenMap(makeSpec({ scheme: "light" }));
    expect(light["color-sunken"]).toBe("#f1f1f4");
  });

  it("explicit elevated / sunken anchors pass through verbatim", () => {
    const tokens = buildTokenMap(
      makeSpec({
        surfaces: { bg: "#111111", surface: "#1a1a1a", elevated: "#222", sunken: "#000" },
      }),
    );
    expect(tokens["color-elevated"]).toBe("#222");
    expect(tokens["color-sunken"]).toBe("#000");
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

  it("keeps the omitted faint ink at the readable muted fallback", () => {
    const tokens = buildTokenMap(makeSpec({ ink: { text: "#eeeeee", textBright: "#ffffff" } }));
    expect(tokens["color-text"]).toBe("#eeeeee");
    expect(tokens["color-text-soft"]).toBe(
      "color-mix(in oklab, var(--color-text) 82%, transparent)",
    );
    expect(tokens["color-text-muted"]).toBe(
      "color-mix(in oklab, var(--color-text) 56%, transparent)",
    );
    expect(tokens["color-text-faint"]).toBe(
      "color-mix(in oklab, var(--color-text) 56%, transparent)",
    );
  });

  it("SCHEME_ICON maps dark/light to moon/sun", () => {
    expect(SCHEME_ICON.dark).toBe("moon");
    expect(SCHEME_ICON.light).toBe("sun");
  });
});
