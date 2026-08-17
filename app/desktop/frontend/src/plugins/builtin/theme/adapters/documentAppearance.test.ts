// The appearance-painter contract: when a preference changes, the document
// reflects it —
//   1. `theme-{scheme}` swaps on <html> from the theme spec's scheme,
//   2. every token in spec.tokens is written to :root.style as an inline var,
//   3. --color-accent resolves from the accent preset + scheme,
//   4. fonts, contrast, radius and motion land as their own custom properties.
//
// This doubles as the contract for theme plugins: register a spec with tokens,
// switch to it, and the DOM reflects the palette. It moved here with the painter
// — the tests belong with the code they pin, and this is no longer a store test.

import { beforeEach, describe, expect, it } from "vitest";
import { ACCENT, COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { useUiStore } from "@/state/uiStore";
import { installDocumentAppearance } from "./documentAppearance";
import { installSystemAppearance } from "./systemAppearance";
import { toggleThemeScheme } from "../application/themeScheme";
import { installThemePreferencePort } from "./uiThemePreference";
import type { PluginContext } from "@/plugins/sdk";
import { contributeForTest } from "@/plugins/sdk/testKernel";

let uninstall: () => void = () => {};
const TEST_MOTION = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  drawerProgress: [0, 0.5, 1],
  pressScale: 0.96,
} as const;

beforeEach(() => {
  // Wipe inline styles + class so each spec starts from a known root.
  document.documentElement.removeAttribute("style");
  document.documentElement.className = "";
  // Reset UI store to defaults (the setup file already wipes plugin store).
  useUiStore.setState({
    theme: "dark",
    visualStyle: "synara",
    accent: "#1ed760",
    contrast: 60,
    uiFont: "",
    codeFont: "",
    fontSize: null,
    fontSmoothing: true,
    radiusScale: 1,
    motionScale: 1,
  });
  // The painter installs from the theme pack's setup in the app; a test drives
  // it directly — in the same order, so resolving a scheme has its port.
  uninstall();
  installThemePreferencePort();
  installSystemAppearance();
  uninstall = installDocumentAppearance(useUiStore);
});

/** Both schemes registered under their canonical ids — anything a test does that
 *  depends on the resolved SCHEME (and not just on the token map) needs them,
 *  because an unregistered id deliberately reads as dark. */
async function registerSchemePair(): Promise<void> {
  await contributeForTest((ctx) => {
    ctx.contribute(COLOR_THEME, { id: "dark", label: "Dark", scheme: "dark" });
    ctx.contribute(COLOR_THEME, { id: "light", label: "Light", scheme: "light" });
  }, "test.schemes");
}

describe("neutral family following the live accent", () => {
  /** How far a hex is from grey, in raw channel spread. */
  function channelSpread(hex: string): number {
    const channels = [1, 3, 5].map((at) => Number.parseInt(hex.slice(at, at + 2), 16));
    return Math.max(...channels) - Math.min(...channels);
  }

  const painted = (name: string) => document.documentElement.style.getPropertyValue(`--${name}`);

  /** A theme that opts in. Dark, so the light-scheme accent remap stays out of it, and
   *  carrying the accent its literals are relative to. */
  function tinted(ctx: PluginContext) {
    ctx.contribute(COLOR_THEME, {
      id: "tinted",
      label: "Tinted",
      scheme: "dark",
      tokens: {
        "color-accent": "#3574f0",
        "color-surface": "#2a2d32",
        "color-sunken": "#14181f",
      },
      neutralSteps: {
        surface: { l: 29.6, c: 0.01 },
        elevated: { l: 29.6, c: 0.01 },
        sunken: { l: 20.9, c: 0.015 },
        border: { l: 35.9, c: 0.0095 },
        borderSoft: { l: 42.3, c: 0.0107 },
      },
    });
  }

  it("reproduces the theme's own literals when the accent is untouched", async () => {
    await contributeForTest((ctx) => {
      tinted(ctx);
    });
    useUiStore.setState({ theme: "tinted", accent: "#3574f0" });

    // The derivation has to be a no-op on the accent the family was measured against,
    // or every golden in the tree moves for nothing.
    expect(painted("color-surface")).toBe("#2a2d32");
    expect(painted("color-sunken")).toBe("#14181f");
  });

  it("goes grey for a grey accent instead of red", async () => {
    await contributeForTest((ctx) => {
      tinted(ctx);
    });
    useUiStore.setState({ theme: "tinted", accent: "#000000" });

    // Reported: picking pure black painted every surface pink, because CSS reads a
    // powerless hue as 0 and 0 is red. A grey accent has no hue to borrow.
    for (const name of ["color-surface", "color-sunken", "color-border"]) {
      expect(channelSpread(painted(name))).toBeLessThanOrEqual(1);
    }
  });

  it("turns the family when the accent changes", async () => {
    await contributeForTest((ctx) => {
      tinted(ctx);
    });
    useUiStore.setState({ theme: "tinted", accent: "#3574f0" });
    const blue = painted("color-sunken");

    useUiStore.setState({ accent: "#e8590c" });
    expect(painted("color-sunken")).not.toBe(blue);
  });

  it("does not touch a palette theme that never opted in", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "solarized",
        label: "Solarized",
        scheme: "light",
        tokens: { "color-accent": "#268bd2", "color-surface": "#eee8d5" },
      });
    });
    useUiStore.setState({ theme: "solarized", accent: "#7f52ff" });

    // Solarized's base2 is Solarized, not a tint of whatever accent is selected.
    expect(painted("color-surface")).toBe("#eee8d5");
  });

  it("collapses the family to neutral at the `off` tint and restores it after", async () => {
    await contributeForTest((ctx) => {
      tinted(ctx);
    });
    useUiStore.setState({ theme: "tinted", accent: "#7f52ff", accentTint: "off" });
    const off = painted("color-sunken");
    expect(channelSpread(off)).toBeLessThanOrEqual(1);

    useUiStore.setState({ accentTint: "standard" });
    expect(painted("color-sunken")).not.toBe(off);
  });
});

describe("applyTheme — theme-as-plugin contract", () => {
  it("writes spec.tokens to :root.style when the active theme is registered", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        tokens: {
          "color-bg": "#101010",
          "color-surface": "#1a1a1a",
        },
      });
    });

    // The registry subscription in uiStore re-fires applyTheme when
    // the themes map mutates, so registering above is enough to write tokens.
    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#101010");
    expect(root.style.getPropertyValue("--color-surface")).toBe("#1a1a1a");
  });

  it("toggles theme-{scheme} class — drives structural CSS overrides", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "solarized-light",
        label: "Solarized Light",
        scheme: "light",
        tokens: { "color-bg": "#fdf6e3" },
      });
    });

    useUiStore.getState().setTheme("solarized-light");

    const root = document.documentElement;
    expect(root.classList.contains("theme-light")).toBe(true);
    expect(root.classList.contains("theme-dark")).toBe(false);
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fdf6e3");
  });

  it("switching themes replaces token values", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        tokens: { "color-bg": "#010102", "color-text": "#f7f8f8" },
      });
      ctx.contribute(COLOR_THEME, {
        id: "light",
        label: "Light",
        scheme: "light",
        tokens: { "color-bg": "#fafafa", "color-text": "#171717" },
      });
    });

    useUiStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-text")).toBe("#171717");

    useUiStore.getState().setTheme("dark");
    expect(root.style.getPropertyValue("--color-bg")).toBe("#010102");
    expect(root.style.getPropertyValue("--color-text")).toBe("#f7f8f8");
  });

  it("removes tokens omitted by the next theme", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        tokens: { "color-bg": "#010102", "color-warning": "#f0c000" },
      });
      ctx.contribute(COLOR_THEME, {
        id: "light",
        label: "Light",
        scheme: "light",
        tokens: { "color-bg": "#fafafa" },
      });
    });

    useUiStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-warning")).toBe("");
  });

  it("resolves accent through the registry for light-scheme themes", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "light",
        label: "Light",
        scheme: "light",
        tokens: {},
      });
      ctx.contribute(ACCENT, {
        id: "green",
        label: "Green",
        dark: "#1ed760",
        light: "#15883e",
      });
    });

    useUiStore.getState().setTheme("light");

    expect(document.documentElement.style.getPropertyValue("--color-accent")).toBe("#15883e");
  });

  it("toggleTheme flips to the first registered theme of the opposite scheme", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        order: 0,
        tokens: {},
      });
      ctx.contribute(COLOR_THEME, {
        id: "solarized-light",
        label: "Solarized Light",
        scheme: "light",
        order: 0,
        tokens: {},
      });
    });

    useUiStore.setState({ theme: "dark" });
    toggleThemeScheme();
    expect(useUiStore.getState().theme).toBe("solarized-light");

    toggleThemeScheme();
    expect(useUiStore.getState().theme).toBe("dark");
  });
});

describe("visual-style contract", () => {
  it("applies component tokens and structural traits independently from colour", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(VISUAL_STYLE, {
        id: "test-style",
        label: "Test style",
        description: "Test",
        traits: { regions: "tool-windows", controls: "outlined" },
        motion: TEST_MOTION,
        preview: {
          canvas: "#fff",
          sidebar: "#eee",
          dock: "#f5f5f5",
          edge: "#ccc",
          accent: "#06f",
        },
        tokens: { "style-shape-md": "5px", "app-content-shadow": "none" },
      });
    });

    useUiStore.getState().setVisualStyle("test-style");

    const root = document.documentElement;
    expect(root.dataset.visualStyle).toBe("test-style");
    expect(root.dataset.regionLayout).toBe("tool-windows");
    expect(root.dataset.controlTreatment).toBe("outlined");
    expect(root.style.getPropertyValue("--style-shape-md")).toBe("5px");
    expect(root.style.getPropertyValue("--app-content-shadow")).toBe("none");
    expect(root.style.getPropertyValue("--dur-fast-base")).toBe("150ms");
    expect(root.style.getPropertyValue("--ease-out")).toBe("cubic-bezier(0.22, 1, 0.36, 1)");
    expect(root.style.getPropertyValue("--ease-drawer")).toBe("linear(0, 0.5, 1)");
  });

  it("removes tokens omitted by the next visual style", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(VISUAL_STYLE, {
        id: "first",
        label: "First",
        description: "First",
        traits: { regions: "floating-card", controls: "quiet" },
        motion: TEST_MOTION,
        preview: {
          canvas: "#fff",
          sidebar: "#eee",
          dock: "#fff",
          edge: "#ccc",
          accent: "#06f",
        },
        tokens: { "shadow-surface-card": "0 8px 20px black" },
      });
      ctx.contribute(VISUAL_STYLE, {
        id: "second",
        label: "Second",
        description: "Second",
        traits: { regions: "flush-panes", controls: "quiet" },
        motion: TEST_MOTION,
        preview: {
          canvas: "#111",
          sidebar: "#222",
          dock: "#111",
          edge: "#333",
          accent: "#69f",
        },
        tokens: {},
      });
    });

    useUiStore.getState().setVisualStyle("first");
    useUiStore.getState().setVisualStyle("second");

    expect(document.documentElement.style.getPropertyValue("--shadow-surface-card")).toBe("");
  });
});

describe("UI preference DOM synchronization", () => {
  it("applies and clears font preferences", async () => {
    const state = useUiStore.getState();
    state.setUiFont("Inter");
    state.setCodeFont("JetBrains Mono");
    state.setFontSize(17);
    state.setFontSmoothing(false);

    const style = document.documentElement.style;
    expect(style.getPropertyValue("--font-sans")).toContain('"Inter"');
    expect(style.getPropertyValue("--font-mono")).toContain('"JetBrains Mono"');
    expect(style.getPropertyValue("-webkit-font-smoothing")).toBe("auto");
    // The base size drives the derived ladder, never the root font-size —
    // scaling <html> would drag every rem-based padding and width with it.
    expect(style.fontSize).toBe("");
    expect(style.getPropertyValue("--fs-ui-md")).toBe("17px");
    expect(style.getPropertyValue("--fs-prose")).toBe("19px");
    expect(style.getPropertyValue("--fs-ui-sm")).toBe("16px");

    useUiStore.getState().setUiFont("");
    useUiStore.getState().setCodeFont("");
    useUiStore.getState().setFontSize(null);

    expect(style.getPropertyValue("--font-sans")).toBe("");
    expect(style.getPropertyValue("--font-mono")).toBe("");
    expect(style.getPropertyValue("--fs-ui-md")).toBe("14px");
  });

  it("applies contrast, radius, and reduced-motion preferences", async () => {
    await registerSchemePair();
    useUiStore.getState().setTheme("light");
    useUiStore.getState().setContrast(100);
    useUiStore.getState().setRadiusScale(1.25);
    useUiStore.getState().setMotionScale(0);

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--depth-step")).toBe("10.0%");
    expect(root.style.getPropertyValue("--radius-scale")).toBe("1.25");
    expect(root.style.getPropertyValue("--motion-scale")).toBe("0");
    expect(root.dataset.motion).toBe("off");

    useUiStore.getState().setMotionScale(0.5);
    expect(root.dataset.motion).toBeUndefined();
  });

  // Equal ink percentages do not buy equal separation: the contrast setting that
  // read right on light collapsed every dark scheme's regions into one value, so
  // the mapping is doubled there.
  it("doubles the ladder step on dark so both schemes separate equally", async () => {
    await registerSchemePair();
    useUiStore.getState().setContrast(25);
    useUiStore.getState().setTheme("light");
    expect(document.documentElement.style.getPropertyValue("--depth-step")).toBe("4.0%");
    useUiStore.getState().setTheme("dark");
    expect(document.documentElement.style.getPropertyValue("--depth-step")).toBe("8.0%");
  });
});
