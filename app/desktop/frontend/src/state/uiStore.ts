// Persisted UI preferences — theme + accent + fonts + message-style +
// sidebar collapse state. Single Zustand store + single persistence key
// since every field is "what the user's UI should look like across
// launches". The side-effects at the bottom of this file mirror the
// active theme spec + font preferences to :root (inline CSS vars +
// theme-{scheme} class on <html>).

import { colord } from "colord";
import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";
// Direct registry import — going through the SDK barrel pulls in
// host.ts which imports this file, creating a TDZ cycle under Vitest.
// Same reason the extension-point reads below import from the deep
// `selectors/extensions` + `kernelPoints` paths (neither pulls host).
import { usePluginStore } from "@/plugins/sdk/registry";
import { ACCENT, THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";

// localStorage payload schema. Validated on rehydrate so a corrupted
// `lyra.ui` entry (manual edit, downgrade leaving a future-shape blob,
// browser extension tampering) falls back to defaults instead of
// crashing the boot.
const uiPersistSchema = z.object({
  theme: z.string(),
  accent: z.string(),
  customTheme: z.object({ bg: z.string(), fg: z.string() }),
  // .default keeps older blobs (no contrast field) parsing — no version bump.
  contrast: z.number().default(60),
  uiFont: z.string(),
  codeFont: z.string(),
  fontSize: z.number().nullable(),
  fontSmoothing: z.boolean(),
  radiusScale: z.number(),
  motionScale: z.number(),
  messageStyle: z.enum(["bubble", "plain"]),
  // .default keeps older blobs (no streamReveal field) parsing — no version bump.
  streamReveal: z.enum(["smooth", "typewriter"]).default("smooth"),
  // .default keeps older blobs (no splitRatio) parsing — no version bump.
  splitRatio: z.number().default(0.5),
  sidebarRail: z.boolean(),
  // .default keeps older blobs (no completionSound) parsing — no version bump.
  completionSound: z.boolean().default(false),
});

/**
 * A theme id — references a `ThemeSpec` registered via
 * `host.extensions.contribute(THEME, …)`. Built-ins ship as `"dark"` / `"light"`
 * + the 8 IDE-style packs; plugins can add more.
 *
 * Code that needs the binary dark/light distinction (shiki / mermaid
 * presets, asset selection) should read the active theme's `scheme`
 * through the theme-scheme resolver rather than
 * comparing the id directly — custom themes like "solarized-dark"
 * would otherwise fall through.
 */
export type Theme = string;

/** The user's editable "custom" theme — background + foreground. The full
 *  palette (surface + ink + border ladders, semantic colors) is derived
 *  from these via the custom-theme plugin; the accent comes from the shared
 *  global `accent` picker (one accent across all themes), and the scheme is
 *  inferred from `bg` luminance. */
export interface CustomTheme {
  bg: string;
  fg: string;
}

interface UiState {
  theme: Theme;
  accent: string;
  /** Edited by the "Custom" theme pickers; applied when `theme === "custom"`. */
  customTheme: CustomTheme;
  /** Global UI contrast 0–100. Drives the surface-ladder depth (`--depth-step`)
   *  across all themes, and the full derived ladders of the custom theme. */
  contrast: number;
  /** Empty string = the native system default. Anything else overrides
   *  `--font-sans` on :root and propagates via Tailwind's `font-sans`. */
  uiFont: string;
  /** Empty string = the native system mono default. */
  codeFont: string;
  /** Pixel font-size on <html>. Null = browser default (16px) so existing
   *  rem tokens stay calibrated. Range: 13–18 in the picker. */
  fontSize: number | null;
  /** macOS-style font antialiasing. true → `-webkit-font-smoothing: antialiased`
   *  (lighter strokes); false → `auto` (the OS default, heavier). */
  fontSmoothing: boolean;
  /** Global radius multiplier — Tailwind 4 `rounded-*` utilities read
   *  `var(--radius-*)` tokens, and each token multiplies by
   *  `--radius-scale`, so any value here propagates to every corner
   *  in the app. 1 = default, < 1 = sharper, > 1 = softer. */
  radiusScale: number;
  /** Global motion multiplier — applies to the CSS `--dur-*` tokens
   *  (CSS keyframes, our `transition` shorthands, KaTeX) AND to the
   *  motion/react presets in lib/motion.ts via the same scale. 0 =
   *  effectively off (blanket 1ms overrides apply), 1 = default. */
  motionScale: number;
  /** "bubble" (default): user messages render as right-aligned cards with
   *  rounded corners + surface-2 fill. "plain": user messages match the
   *  assistant's flat layout — left-aligned, no bubble. */
  messageStyle: "bubble" | "plain";
  /** Streaming reveal style for assistant replies. "smooth" (default):
   *  word-by-word reveal with a per-word fade-in. "typewriter": crisp
   *  character-by-character reveal, no fade — a steady terminal cadence. */
  streamReveal: "smooth" | "typewriter";
  /** Chat-vs-side-pane split: the fraction (0.25–0.75) of the main area the
   *  chat stream takes when a `splitViewId` side pane is open. */
  splitRatio: number;
  /** True = collapsed rail. False = expanded sidebar. */
  sidebarRail: boolean;
  /** Play a soft chime when a run settles while the window is unfocused —
   *  the audible companion to the OS completion notification. Default off. */
  completionSound: boolean;
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
  /** Patch one or more of the custom theme's base colors. */
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
  setContrast: (contrast: number) => void;
  setUiFont: (font: string) => void;
  setCodeFont: (font: string) => void;
  setFontSize: (size: number | null) => void;
  setFontSmoothing: (on: boolean) => void;
  setRadiusScale: (scale: number) => void;
  setMotionScale: (scale: number) => void;
  setMessageStyle: (style: "bubble" | "plain") => void;
  setStreamReveal: (mode: "smooth" | "typewriter") => void;
  setSplitRatio: (ratio: number) => void;
  toggleSidebar: () => void;
  setCompletionSound: (on: boolean) => void;
}

export const useUiStore = create<UiState & UiActions>()(
  persist(
    (set, get) => ({
      theme: "system",
      accent: "#6c97ff",
      customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
      contrast: 60,
      uiFont: "",
      codeFont: "",
      fontSize: null,
      fontSmoothing: true,
      radiusScale: 1,
      motionScale: 1,
      messageStyle: "bubble",
      streamReveal: "smooth",
      splitRatio: 0.5,
      sidebarRail: true,
      completionSound: false,

      setTheme: (theme) => set({ theme }),
      toggleTheme: () => {
        const cur = resolveThemeId(get().theme);
        const curSpec = lookupExtensionByKey(THEME, cur);
        const curScheme = curSpec?.scheme ?? (cur === "light" ? "light" : "dark");
        const target = curScheme === "dark" ? "light" : "dark";
        // `lookupExtensionPoint` returns themes already sorted by `order`, so
        // the first match is the "primary" theme of the opposite scheme —
        // matches the sort the appearance pane uses.
        const candidates = lookupExtensionPoint(THEME).filter((t) => t.scheme === target);
        if (candidates[0]) set({ theme: candidates[0].id });
      },
      setAccent: (accent) => set({ accent }),
      setCustomTheme: (patch) => set((s) => ({ customTheme: { ...s.customTheme, ...patch } })),
      setContrast: (contrast) => set({ contrast }),
      setUiFont: (uiFont) => set({ uiFont }),
      setCodeFont: (codeFont) => set({ codeFont }),
      setFontSize: (fontSize) => set({ fontSize }),
      setFontSmoothing: (fontSmoothing) => set({ fontSmoothing }),
      setRadiusScale: (radiusScale) => set({ radiusScale }),
      setMotionScale: (motionScale) => set({ motionScale }),
      setMessageStyle: (messageStyle) => set({ messageStyle }),
      setStreamReveal: (streamReveal) => set({ streamReveal }),
      setSplitRatio: (splitRatio) => set({ splitRatio }),
      toggleSidebar: () => set((s) => ({ sidebarRail: !s.sidebarRail })),
      setCompletionSound: (completionSound) => set({ completionSound }),
    }),
    {
      name: "lyra.ui",
      storage: createJSONStorage(() => localStorage),
      version: 4,
      merge: (persisted, current) => {
        const parsed = uiPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          // Reset on schema mismatch — defaults are always a safe boot.
          console.warn("[uiStore] discarding corrupted lyra.ui:", parsed.error.issues);
          return current;
        }
        return { ...current, ...parsed.data };
      },
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
  const match = lookupExtensionPoint(ACCENT).find((a) => a.dark === darkHex);
  if (match) return match.light ?? darkHex;
  return colord(darkHex).darken(0.2).toHex();
}

// `theme: "system"` follows the OS appearance — resolve it to the primary
// theme of the current prefers-color-scheme. Any other id is used as-is.
function prefersDark(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches
  );
}
function resolveThemeId(theme: Theme): Theme {
  return theme === "system" ? (prefersDark() ? "dark" : "light") : theme;
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
  const resolved = resolveThemeId(theme);
  const spec = lookupExtensionByKey(THEME, resolved);

  // Scheme drives the structural class. Fallback to id when the spec
  // isn't registered yet — for built-in ids ("dark"/"light") still right.
  const scheme = spec?.scheme ?? (resolved === "light" ? "light" : "dark");
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
  //
  // Exception: the surface-2/3/4 ladder is NEVER written inline, even when a
  // theme pins it — we let globals.css derive it via color-mix(--depth-step)
  // so the global contrast slider moves the ladder for EVERY theme (8 of 10
  // built-ins pin these; pinning would make contrast a no-op for them). The
  // theme's identity colors — bg / surface / text / accent — still win.
  if (spec?.tokens) {
    for (const [name, value] of Object.entries(spec.tokens)) {
      if (name === "color-surface-2" || name === "color-surface-3" || name === "color-surface-4") {
        continue;
      }
      const fullName = `--${name}`;
      root.style.setProperty(fullName, value);
      appliedTokenNames.push(fullName);
    }
  }

  // Accent override last so the user's accent pick beats the theme's
  // default --color-accent token (applies to every theme, custom included —
  // one shared accent).
  const c = scheme === "light" ? lookupLightVariant(accent) : accent;
  root.style.setProperty("--color-accent", c);
  if (!appliedTokenNames.includes("--color-accent")) {
    appliedTokenNames.push("--color-accent");
  }

  // Global contrast → surface-ladder depth. Overrides the theme's depth-step
  // token for every theme (2%..10%, default 60 ≈ 6.8%); the custom theme's
  // fuller ladders also read `contrast` (in the custom-theme plugin).
  const contrast = useUiStore.getState().contrast;
  root.style.setProperty("--depth-step", `${(2 + (contrast / 100) * 8).toFixed(1)}%`);
  if (!appliedTokenNames.includes("--depth-step")) {
    appliedTokenNames.push("--depth-step");
  }
}

// Apply user font preferences to :root. uiFont / codeFont override the
// Tailwind --font-sans / --font-mono tokens; fontSize sets <html>
// font-size in px so every rem-anchored token scales proportionally.
// (See tokens.css comment for why we DON'T set this to a rem-based
// value — that would self-reference.)
function applyFonts(
  uiFont: string,
  codeFont: string,
  fontSize: number | null,
  fontSmoothing: boolean,
) {
  const root = document.documentElement;
  // antialiased = lighter macOS-style strokes; auto = OS default (heavier).
  root.style.setProperty("-webkit-font-smoothing", fontSmoothing ? "antialiased" : "auto");
  root.style.setProperty("-moz-osx-font-smoothing", fontSmoothing ? "grayscale" : "auto");
  if (uiFont) {
    root.style.setProperty(
      "--font-sans",
      `"${uiFont}", -apple-system, system-ui, "PingFang SC", sans-serif`,
    );
  } else {
    root.style.removeProperty("--font-sans");
  }
  if (codeFont) {
    root.style.setProperty(
      "--font-mono",
      `"${codeFont}", ui-monospace, "SF Mono", Menlo, monospace`,
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

// Apply shape + motion preferences to :root. Both are scalar CSS vars
// that ripple through Tailwind 4's `@theme inline` token bridge — so
// `--radius-scale` instantly re-rounds every `rounded-*` utility in
// the app, and `--motion-scale` re-paces every `--dur-*` token in the
// app's CSS-controlled animations. lib/motion.ts also reads
// `motionScale` from this store for the motion/react preset durations.
function applyShape(radiusScale: number, motionScale: number) {
  const root = document.documentElement;
  root.style.setProperty("--radius-scale", String(radiusScale));
  root.style.setProperty("--motion-scale", String(motionScale));
  // `data-motion="off"` triggers the blanket transition/animation
  // override in globals.css so even Tailwind's literal-ms durations
  // (which can't be multiplied via a CSS var) collapse to ~0.
  if (motionScale === 0) root.setAttribute("data-motion", "off");
  else root.removeAttribute("data-motion");
}

// Persist middleware rehydrates synchronously on store creation, so
// getState() already reflects persisted values.
{
  const s = useUiStore.getState();
  applyTheme(s.theme, s.accent);
  applyFonts(s.uiFont, s.codeFont, s.fontSize, s.fontSmoothing);
  applyShape(s.radiusScale, s.motionScale);
}
const unsubUi = useUiStore.subscribe((state, prev) => {
  if (
    state.theme !== prev.theme ||
    state.accent !== prev.accent ||
    state.contrast !== prev.contrast
  ) {
    applyTheme(state.theme, state.accent);
  }
  if (
    state.uiFont !== prev.uiFont ||
    state.codeFont !== prev.codeFont ||
    state.fontSize !== prev.fontSize ||
    state.fontSmoothing !== prev.fontSmoothing
  ) {
    applyFonts(state.uiFont, state.codeFont, state.fontSize, state.fontSmoothing);
  }
  if (state.radiusScale !== prev.radiusScale || state.motionScale !== prev.motionScale) {
    applyShape(state.radiusScale, state.motionScale);
  }
});

// Re-apply when the plugin registry mutates — handles built-in themes
// registering after the initial applyTheme call (empty registry) and
// runtime hot-loading of theme plugins.
const unsubPlugins = usePluginStore.subscribe((state, prev) => {
  // Theme + accent live on the shared `extensions` map now, so re-apply on
  // any registry mutation. Mutations only happen at plugin load/unload (not
  // during streaming), so this stays quiet at steady state.
  if (state.extensions !== prev.extensions) {
    const { theme, accent } = useUiStore.getState();
    applyTheme(theme, accent);
  }
});

// Follow the OS appearance while theme === "system": re-apply on scheme flip
// so a system-mode user tracks light/dark live, without a manual toggle.
let unsubScheme = () => {};
if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
  const mq = window.matchMedia("(prefers-color-scheme: dark)");
  const onSchemeChange = () => {
    if (useUiStore.getState().theme === "system") {
      const { theme, accent } = useUiStore.getState();
      applyTheme(theme, accent);
    }
  };
  mq.addEventListener("change", onSchemeChange);
  unsubScheme = () => mq.removeEventListener("change", onSchemeChange);
}

disposeOnHmr(unsubUi, unsubPlugins, unsubScheme);
