// Theme + accent — the appearance slice of the kernel's UI state.
//
// Persisted: `theme` (id from the plugin registry) and `accent` (hex).
// The active theme + accent are mirrored to :root via the side-effects
// at the bottom of this file: tokens become inline CSS vars, the
// scheme class on <html> toggles, accent picks up the right light/dark
// variant.

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { Theme } from "@/components/sidebar/types";
// Import the registry store directly rather than via the SDK barrel —
// the barrel pulls in definePlugin / host, and host.ts already imports
// this file. Going through the barrel creates a real cycle that shows
// up as a TDZ at module-init time under the Vitest loader.
import { usePluginStore } from "@/plugins/sdk/registry";

type ThemeState = {
  theme: Theme;
  accent: string;
};

type ThemeActions = {
  setTheme: (theme: Theme) => void;
  /**
   * Flip to the opposite SCHEME (not just "dark"/"light" id) so custom
   * theme plugins still toggle sensibly. Picks the first registered
   * theme whose scheme is the opposite of the current one; no-op if
   * none exists (e.g. only dark themes registered).
   */
  toggleTheme: () => void;
  setAccent: (accent: string) => void;
};

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

// ---------------------------------------------------------------------------
// Side-effects: keep <html> class + inline CSS vars in sync with the
// active theme spec from the plugin registry.
// ---------------------------------------------------------------------------
//
// Theme model — IDE/VS Code style:
//   1. A theme plugin (default: `lyra.builtin.theme-dark` etc.) registers
//      one or more ThemeSpec entries. Each carries a `tokens` map: CSS
//      variable name → value.
//   2. When `theme` changes (or the registry's theme map mutates because
//      a plugin registered late), we look up the spec, toggle the
//      `theme-{scheme}` class on <html> so structural CSS still applies,
//      and write every token to `:root.style` as an inline override.
//   3. Until the plugin registers, the tokens declared in `tokens.css`
//      (`:root`) take effect as a first-paint fallback. The fallback
//      values match the dark theme — light users see a brief dark flash
//      before the plugin registers and inline tokens kick in.
//
// Accent works the same way: the accent picker stores a hex; we resolve
// to the light variant via the accent registry when the active theme's
// scheme is "light".

function lookupLightVariant(darkHex: string): string | undefined {
  const accents = usePluginStore.getState().accents;
  for (const o of accents.values()) {
    if (o.value.dark === darkHex) return o.value.light ?? darkHex;
  }
  return undefined;
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
  const c = scheme === "light" ? (lookupLightVariant(accent) ?? accent) : accent;
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
