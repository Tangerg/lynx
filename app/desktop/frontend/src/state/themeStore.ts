// Theme + accent store. Persists the theme id + accent hex; the
// side-effects at the bottom of this file mirror the active spec to
// :root (inline CSS vars + theme-{scheme} class on <html>).

import type { Theme } from "@/components/sidebar/types";
import { colord } from "colord";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
// Direct registry import — going through the SDK barrel pulls in
// host.ts which imports this file, creating a TDZ cycle under Vitest.
import { usePluginStore } from "@/plugins/sdk/registry";

interface ThemeState {
  theme: Theme;
  accent: string;
}

interface ThemeActions {
  setTheme: (theme: Theme) => void;
  /**
   * Flip to the opposite SCHEME (not just "dark"/"light" id) so custom
   * theme plugins still toggle sensibly. Picks the first registered
   * theme whose scheme is the opposite of the current one; no-op if
   * none exists (e.g. only dark themes registered).
   */
  toggleTheme: () => void;
  setAccent: (accent: string) => void;
}

export const useThemeStore = create<ThemeState & ThemeActions>()(
  persist(
    (set, get) => ({
      theme: "dark",
      accent: "#1ed760",

      setTheme: (theme) => set({ theme }),
      toggleTheme: () => {
        const cur = get().theme;
        const themes = usePluginStore.getState().themes;
        const curSpec = themes.get(cur)?.value;
        const curScheme = curSpec?.scheme ?? (cur === "light" ? "light" : "dark");
        const target = curScheme === "dark" ? "light" : "dark";
        // Sort by `order` so the toggle picks the "primary" theme of the
        // opposite scheme rather than whichever Map happens to enumerate
        // first. Matches the sort the appearance pane uses.
        const candidates = Array.from(themes.values())
          .map((o) => o.value)
          .filter((t) => t.scheme === target)
          .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
        if (candidates[0]) set({ theme: candidates[0].id });
      },
      setAccent: (accent) => set({ accent }),
    }),
    {
      name: "lyra.theme",
      storage: createJSONStorage(() => localStorage),
      version: 1,
    },
  ),
);

// Side-effects below mirror the active theme + accent to :root. Until
// the theme plugin registers, tokens.css :root values stand in as
// first-paint fallback.

// Resolve the accent's light-theme variant. Registered presets have a
// hand-tuned `light` hex; custom colors (via the picker) don't, so we
// darken the dark hex by ~20% — enough to give it legible contrast on
// the bright surface without losing the user's chosen hue.
function lookupLightVariant(darkHex: string): string {
  const accents = usePluginStore.getState().accents;
  for (const o of accents.values()) {
    if (o.value.dark === darkHex) return o.value.light ?? darkHex;
  }
  return colord(darkHex).darken(0.2).toHex();
}

// Track every CSS variable the last theme set on :root.style so we can
// remove it before applying the next theme. Without this, a theme that
// pins surface-2/3/4 (Catppuccin, Tokyo Night, Atom One) leaves those
// values on :root after the user switches to a theme that doesn't pin
// them — and the new theme's color-mix() fallbacks never get a chance
// to kick in because the old inline property still wins.
let appliedTokenNames: string[] = [];

function applyTheme(theme: Theme, accent: string) {
  const root = document.documentElement;
  const spec = usePluginStore.getState().themes.get(theme)?.value;

  // Scheme drives the structural class. Fallback to id when the spec
  // isn't registered yet — for built-in ids ("dark"/"light") still right.
  const scheme = spec?.scheme ?? (theme === "light" ? "light" : "dark");
  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  // Step 1 — clear every token the previous theme wrote. Anything the
  // new theme also sets will be re-added in step 2; anything it doesn't
  // falls through to tokens.css's :root defaults (and the color-mix()
  // surface ladder derivations).
  for (const name of appliedTokenNames) {
    root.style.removeProperty(name);
  }
  appliedTokenNames = [];

  // Step 2 — write the new theme's tokens. Inline beats stylesheet, so
  // the plugin owns the palette regardless of what tokens.css says.
  if (spec?.tokens) {
    for (const [name, value] of Object.entries(spec.tokens)) {
      const fullName = `--${name}`;
      root.style.setProperty(fullName, value);
      appliedTokenNames.push(fullName);
    }
  }

  // Accent override last so the user's accent pick beats the theme's
  // default --color-accent token. Also tracked so a theme switch clears
  // it cleanly.
  const c = scheme === "light" ? lookupLightVariant(accent) : accent;
  root.style.setProperty("--color-accent", c);
  if (!appliedTokenNames.includes("--color-accent")) {
    appliedTokenNames.push("--color-accent");
  }
}

// Persist middleware rehydrates synchronously on store creation, so
// getState() already reflects persisted values.
applyTheme(useThemeStore.getState().theme, useThemeStore.getState().accent);
useThemeStore.subscribe((state, prev) => {
  if (state.theme !== prev.theme || state.accent !== prev.accent) {
    applyTheme(state.theme, state.accent);
  }
});

// Re-apply when the plugin registry mutates — handles built-in themes
// registering after the initial applyTheme call (empty registry) and
// runtime hot-loading of theme plugins.
usePluginStore.subscribe((state, prev) => {
  if (state.themes !== prev.themes || state.accents !== prev.accents) {
    const { theme, accent } = useThemeStore.getState();
    applyTheme(theme, accent);
  }
});
