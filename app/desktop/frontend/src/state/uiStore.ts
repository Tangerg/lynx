// Persisted UI preferences — theme + accent + fonts + message-style +
// sidebar collapse state. Single Zustand store + single persistence key
// since every field is "what the user's UI should look like across
// launches". The side-effects at the bottom of this file mirror the
// active theme spec + font preferences to :root (inline CSS vars +
// theme-{scheme} class on <html>).

import { colord } from "colord";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
// Direct registry import — going through the SDK barrel pulls in
// host.ts which imports this file, creating a TDZ cycle under Vitest.
import { usePluginStore } from "@/plugins/sdk/registry";

/**
 * A theme id — references a `ThemeSpec` registered via
 * `host.theme.registerTheme()`. Built-ins ship as `"dark"` / `"light"`
 * + the 8 IDE-style packs; plugins can add more.
 *
 * Code that needs the binary dark/light distinction (shiki / mermaid
 * presets, asset selection) should read the active theme's `scheme`
 * via `resolveScheme(themeId)` from `@/plugins/sdk` rather than
 * comparing the id directly — custom themes like "solarized-dark"
 * would otherwise fall through.
 */
export type Theme = string;

interface UiState {
  theme: Theme;
  accent: string;
  /** Empty string = use the bundled Geist default. Anything else overrides
   *  `--font-sans` on :root and propagates via Tailwind's `font-sans`. */
  uiFont: string;
  /** Empty string = use the bundled Geist Mono default. */
  codeFont: string;
  /** Pixel font-size on <html>. Null = browser default (16px) so existing
   *  rem tokens stay calibrated. Range: 13–18 in the picker. */
  fontSize: number | null;
  /** "bubble" (default): user messages render as right-aligned cards with
   *  rounded corners + surface-2 fill. "plain": user messages match the
   *  assistant's flat layout — left-aligned, no bubble. */
  messageStyle: "bubble" | "plain";
  /** True = collapsed rail. False = expanded sidebar. */
  sidebarRail: boolean;
}

interface UiActions {
  setTheme: (theme: Theme) => void;
  /**
   * Flip to the opposite SCHEME (not just "dark"/"light" id) so custom
   * theme plugins still toggle sensibly. Picks the first registered
   * theme whose scheme is the opposite of the current one; no-op if
   * none exists (e.g. only dark themes registered).
   */
  toggleTheme: () => void;
  setAccent: (accent: string) => void;
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setMessageStyle: (style: "bubble" | "plain") => void;
  toggleSidebar: () => void;
}

export const useUiStore = create<UiState & UiActions>()(
  persist(
    (set, get) => ({
      theme: "dark",
      accent: "#1ed760",
      uiFont: "",
      codeFont: "",
      fontSize: null,
      messageStyle: "bubble",
      sidebarRail: true,

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
      setUiFont: (uiFont) => set({ uiFont }),
      setCodeFont: (codeFont) => set({ codeFont }),
      setFontSize: (fontSize) => set({ fontSize }),
      setMessageStyle: (messageStyle) => set({ messageStyle }),
      toggleSidebar: () => set((s) => ({ sidebarRail: !s.sidebarRail })),
    }),
    {
      name: "lyra.ui",
      storage: createJSONStorage(() => localStorage),
      version: 1,
    },
  ),
);

// Side-effects below mirror the active theme + accent + fonts to :root.
// Until the theme plugin registers, tokens.css :root values stand in as
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

// Apply user font preferences to :root. uiFont / codeFont override the
// Tailwind --font-sans / --font-mono tokens; fontSize sets <html>
// font-size in px so every rem-anchored token scales proportionally.
// (See tokens.css comment for why we DON'T set this to a rem-based
// value — that would self-reference.)
function applyFonts(uiFont: string, codeFont: string, fontSize: number | null) {
  const root = document.documentElement;
  if (uiFont) {
    root.style.setProperty("--font-sans", `"${uiFont}", "Geist", "Inter", system-ui, sans-serif`);
  } else {
    root.style.removeProperty("--font-sans");
  }
  if (codeFont) {
    root.style.setProperty(
      "--font-mono",
      `"${codeFont}", "Geist Mono", "JetBrains Mono", ui-monospace, Menlo, monospace`,
    );
  } else {
    root.style.removeProperty("--font-mono");
  }
  if (fontSize) {
    root.style.fontSize = `${fontSize}px`;
  } else {
    root.style.fontSize = "";
  }
}

// Persist middleware rehydrates synchronously on store creation, so
// getState() already reflects persisted values.
{
  const s = useUiStore.getState();
  applyTheme(s.theme, s.accent);
  applyFonts(s.uiFont, s.codeFont, s.fontSize);
}
const unsubUi = useUiStore.subscribe((state, prev) => {
  if (state.theme !== prev.theme || state.accent !== prev.accent) {
    applyTheme(state.theme, state.accent);
  }
  if (
    state.uiFont !== prev.uiFont ||
    state.codeFont !== prev.codeFont ||
    state.fontSize !== prev.fontSize
  ) {
    applyFonts(state.uiFont, state.codeFont, state.fontSize);
  }
});

// Re-apply when the plugin registry mutates — handles built-in themes
// registering after the initial applyTheme call (empty registry) and
// runtime hot-loading of theme plugins.
const unsubPlugins = usePluginStore.subscribe((state, prev) => {
  if (state.themes !== prev.themes || state.accents !== prev.accents) {
    const { theme, accent } = useUiStore.getState();
    applyTheme(theme, accent);
  }
});

// HMR safety: this module's two subscribe calls live at module scope,
// not inside an effect with a cleanup. Without an explicit dispose,
// every HMR reload registers a fresh pair of listeners on top of the
// previous ones — after N edits there are N+1 of each firing on every
// theme / accent / plugin mutation, snowballing into visible jank.
if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    unsubUi();
    unsubPlugins();
  });
}
