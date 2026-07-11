// Theme-application contract. Verifies the side effect that lives at the
// bottom of uiStore.ts — when the active theme id changes, the kernel:
//   1. swaps `theme-{scheme}` on <html> based on the theme spec's scheme,
//   2. writes every token from spec.tokens to :root.style as inline vars,
//   3. updates --color-accent based on the resolved accent + scheme.
//
// These tests act as the contract for theme plugins: register a theme
// spec with tokens, switch to it, and the DOM reflects the palette.

import type { Disposable } from "@/plugins/sdk/types";
import { beforeEach, describe, expect, it } from "vitest";
import { createHost } from "@/plugins/sdk/host";
import { ACCENT, THEME } from "@/plugins/sdk/kernelPoints";
import { useUiStore } from "@/state/uiStore";

const sink: Disposable[] = [];

beforeEach(() => {
  // Wipe inline styles + class so each spec starts from a known root.
  document.documentElement.removeAttribute("style");
  document.documentElement.className = "";
  // Reset UI store to defaults (the setup file already wipes plugin store).
  useUiStore.setState({
    theme: "dark",
    accent: "#1ed760",
    contrast: 60,
    uiFont: "",
    codeFont: "",
    fontSize: null,
    fontSmoothing: true,
    radiusScale: 1,
    motionScale: 1,
  });
  sink.length = 0;
});

describe("applyTheme — theme-as-plugin contract", () => {
  it("writes spec.tokens to :root.style when the active theme is registered", () => {
    const host = createHost("test", sink);
    host.extensions.contribute(THEME, {
      id: "dark",
      label: "Dark",
      scheme: "dark",
      tokens: {
        "color-bg": "#101010",
        "color-surface": "#1a1a1a",
      },
    });

    // The registry subscription in uiStore re-fires applyTheme when
    // the themes map mutates, so registering above is enough to write tokens.
    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#101010");
    expect(root.style.getPropertyValue("--color-surface")).toBe("#1a1a1a");
  });

  it("toggles theme-{scheme} class — drives structural CSS overrides", () => {
    const host = createHost("test", sink);
    host.extensions.contribute(THEME, {
      id: "solarized-light",
      label: "Solarized Light",
      scheme: "light",
      tokens: { "color-bg": "#fdf6e3" },
    });

    useUiStore.getState().setTheme("solarized-light");

    const root = document.documentElement;
    expect(root.classList.contains("theme-light")).toBe(true);
    expect(root.classList.contains("theme-dark")).toBe(false);
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fdf6e3");
  });

  it("switching themes replaces token values", () => {
    const host = createHost("test", sink);
    host.extensions.contribute(THEME, {
      id: "dark",
      label: "Dark",
      scheme: "dark",
      tokens: { "color-bg": "#010102", "color-text": "#f7f8f8" },
    });
    host.extensions.contribute(THEME, {
      id: "light",
      label: "Light",
      scheme: "light",
      tokens: { "color-bg": "#fafafa", "color-text": "#171717" },
    });

    useUiStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-text")).toBe("#171717");

    useUiStore.getState().setTheme("dark");
    expect(root.style.getPropertyValue("--color-bg")).toBe("#010102");
    expect(root.style.getPropertyValue("--color-text")).toBe("#f7f8f8");
  });

  it("removes tokens omitted by the next theme", () => {
    const host = createHost("test", sink);
    host.extensions.contribute(THEME, {
      id: "dark",
      label: "Dark",
      scheme: "dark",
      tokens: { "color-bg": "#010102", "color-warning": "#f0c000" },
    });
    host.extensions.contribute(THEME, {
      id: "light",
      label: "Light",
      scheme: "light",
      tokens: { "color-bg": "#fafafa" },
    });

    useUiStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-warning")).toBe("");
  });

  it("resolves accent through the registry for light-scheme themes", () => {
    const host = createHost("test", sink);
    host.extensions.contribute(THEME, {
      id: "light",
      label: "Light",
      scheme: "light",
      tokens: {},
    });
    host.extensions.contribute(ACCENT, {
      id: "green",
      label: "Green",
      dark: "#1ed760",
      light: "#15883e",
    });

    useUiStore.getState().setTheme("light");

    expect(document.documentElement.style.getPropertyValue("--color-accent")).toBe("#15883e");
  });

  it("toggleTheme flips to the first registered theme of the opposite scheme", () => {
    const host = createHost("test", sink);
    host.extensions.contribute(THEME, {
      id: "dark",
      label: "Dark",
      scheme: "dark",
      order: 0,
      tokens: {},
    });
    host.extensions.contribute(THEME, {
      id: "solarized-light",
      label: "Solarized Light",
      scheme: "light",
      order: 0,
      tokens: {},
    });

    useUiStore.setState({ theme: "dark" });
    useUiStore.getState().toggleTheme();
    expect(useUiStore.getState().theme).toBe("solarized-light");

    useUiStore.getState().toggleTheme();
    expect(useUiStore.getState().theme).toBe("dark");
  });
});

describe("UI preference DOM synchronization", () => {
  it("applies and clears font preferences", () => {
    const state = useUiStore.getState();
    state.setUiFont("Inter");
    state.setCodeFont("JetBrains Mono");
    state.setFontSize(17);
    state.setFontSmoothing(false);

    const style = document.documentElement.style;
    expect(style.getPropertyValue("--font-sans")).toContain('"Inter"');
    expect(style.getPropertyValue("--font-mono")).toContain('"JetBrains Mono"');
    expect(style.fontSize).toBe("17px");
    expect(style.getPropertyValue("-webkit-font-smoothing")).toBe("auto");

    useUiStore.getState().setUiFont("");
    useUiStore.getState().setCodeFont("");
    useUiStore.getState().setFontSize(null);

    expect(style.getPropertyValue("--font-sans")).toBe("");
    expect(style.getPropertyValue("--font-mono")).toBe("");
    expect(style.fontSize).toBe("");
  });

  it("applies contrast, radius, and reduced-motion preferences", () => {
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
});
