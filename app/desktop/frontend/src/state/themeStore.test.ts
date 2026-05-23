// Theme-application contract. Verifies the side effect that lives at the
// bottom of themeStore.ts — when the active theme id changes, the kernel:
//   1. swaps `theme-{scheme}` on <html> based on the theme spec's scheme,
//   2. writes every token from spec.tokens to :root.style as inline vars,
//   3. updates --color-accent based on the resolved accent + scheme.
//
// These tests act as the contract for theme plugins: register a theme
// spec with tokens, switch to it, and the DOM reflects the palette.

import { describe, expect, it, beforeEach } from "vitest";
import { createHost } from "@/plugins/sdk/host";
import type { Disposable } from "@/plugins/sdk/types";
import { useThemeStore } from "@/state/themeStore";

const sink: Disposable[] = [];

beforeEach(() => {
  // Wipe inline styles + class so each spec starts from a known root.
  document.documentElement.removeAttribute("style");
  document.documentElement.className = "";
  // Reset theme store to defaults (the setup file already wipes plugin store).
  useThemeStore.setState({ theme: "dark", accent: "#1ed760" });
  sink.length = 0;
});

describe("applyTheme — theme-as-plugin contract", () => {
  it("writes spec.tokens to :root.style when the active theme is registered", () => {
    const host = createHost("test", sink);
    host.theme.registerTheme({
      id: "dark",
      label: "Dark",
      scheme: "dark",
      tokens: {
        "color-bg": "#101010",
        "color-surface": "#1a1a1a",
      },
    });

    // The registry subscription in themeStore re-fires applyTheme when
    // the themes map mutates, so registering above is enough to write tokens.
    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#101010");
    expect(root.style.getPropertyValue("--color-surface")).toBe("#1a1a1a");
  });

  it("toggles theme-{scheme} class — drives structural CSS overrides", () => {
    const host = createHost("test", sink);
    host.theme.registerTheme({
      id: "solarized-light",
      label: "Solarized Light",
      scheme: "light",
      tokens: { "color-bg": "#fdf6e3" },
    });

    useThemeStore.getState().setTheme("solarized-light");

    const root = document.documentElement;
    expect(root.classList.contains("theme-light")).toBe(true);
    expect(root.classList.contains("theme-dark")).toBe(false);
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fdf6e3");
  });

  it("switching themes replaces token values", () => {
    const host = createHost("test", sink);
    host.theme.registerTheme({
      id: "dark",
      label: "Dark",
      scheme: "dark",
      tokens: { "color-bg": "#010102", "color-text": "#f7f8f8" },
    });
    host.theme.registerTheme({
      id: "light",
      label: "Light",
      scheme: "light",
      tokens: { "color-bg": "#fafafa", "color-text": "#171717" },
    });

    useThemeStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-text")).toBe("#171717");

    useThemeStore.getState().setTheme("dark");
    expect(root.style.getPropertyValue("--color-bg")).toBe("#010102");
    expect(root.style.getPropertyValue("--color-text")).toBe("#f7f8f8");
  });

  it("resolves accent through the registry for light-scheme themes", () => {
    const host = createHost("test", sink);
    host.theme.registerTheme({
      id: "light",
      label: "Light",
      scheme: "light",
      tokens: {},
    });
    host.theme.registerAccent({
      id: "green",
      label: "Green",
      dark: "#1ed760",
      light: "#15883e",
    });

    useThemeStore.getState().setTheme("light");

    expect(document.documentElement.style.getPropertyValue("--color-accent"))
      .toBe("#15883e");
  });

  it("toggleTheme flips to the first registered theme of the opposite scheme", () => {
    const host = createHost("test", sink);
    host.theme.registerTheme({
      id: "dark",
      label: "Dark",
      scheme: "dark",
      order: 0,
      tokens: {},
    });
    host.theme.registerTheme({
      id: "solarized-light",
      label: "Solarized Light",
      scheme: "light",
      order: 0,
      tokens: {},
    });

    useThemeStore.setState({ theme: "dark" });
    useThemeStore.getState().toggleTheme();
    expect(useThemeStore.getState().theme).toBe("solarized-light");

    useThemeStore.getState().toggleTheme();
    expect(useThemeStore.getState().theme).toBe("dark");
  });
});
