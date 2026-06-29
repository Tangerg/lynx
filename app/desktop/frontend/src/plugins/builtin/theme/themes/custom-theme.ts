// Built-in plugin: the user-editable "Custom" theme.
//
// A custom theme is just three base colors (accent / bg / fg, in
// uiStore.customTheme). The full palette — surface + ink + border ladders,
// semantic colors — is DERIVED here so the user only ever touches three
// pickers (the Codex/Linear model). Derivation uses CSS `color-mix()` for
// the ladders (the browser resolves them against bg/fg at paint time), so we
// don't hand-compute 15 hex values; `scheme` is inferred from bg luminance.
//
// The theme registers under id "custom" like any other, so the picker,
// scheme resolution, and applyTheme treat it uniformly. It re-registers on
// every customTheme change → the registry mutates → uiStore re-applies it →
// live preview while dragging the pickers.

import { colord } from "colord";
import { disposeOnHmr } from "@/lib/hmr";
import { definePlugin } from "@/plugins/sdk";
import { THEME } from "@/plugins/sdk/kernelPoints";
import { useUiStore } from "@/state/uiStore";
import { buildTokenMap } from "../kit/tokens";
import type { ThemePluginSpec } from "../kit/types";
import type { CustomTheme } from "@/state/uiStore";

const CUSTOM_THEME_ID = "custom";

// mix(a, pct, b) → CSS color-mix string: pct% of `a`, rest `b`. Resolved by
// the browser, so the derived ladder tracks the base colors exactly.
const mix = (a: string, pct: number, b: string): string =>
  `color-mix(in oklab, ${a} ${pct}%, ${b})`;

/** Derive a full theme spec from the custom bg/fg + the shared global accent.
 *  `contrast` (0–100) scales how far each derived ladder spreads from the
 *  base colors — low = flat/subtle, high = punchy. */
function deriveCustomSpec(ct: CustomTheme, accent: string, contrast: number): ThemePluginSpec {
  const { bg, fg } = ct;
  const k = Math.min(100, Math.max(0, contrast)) / 100; // 0..1 — global contrast
  // lerp a fg-toward-bg mix percentage by contrast, then round to an int.
  const p = (lo: number, hi: number) => Math.round(lo + (hi - lo) * k);
  const scheme: "dark" | "light" = colord(bg).isDark() ? "dark" : "light";
  const extreme = scheme === "dark" ? "#ffffff" : "#000000";
  return {
    id: CUSTOM_THEME_ID,
    label: "Custom",
    scheme,
    // (--depth-step is set globally from contrast in uiStore.applyTheme)
    brand: { accent, textOnAccent: colord(accent).isDark() ? "#ffffff" : "#000000" },
    surfaces: { bg, surface: mix(fg, p(4, 12), bg) },
    ink: {
      text: fg,
      textBright: mix(fg, 80, extreme), // nudge toward pure white/black
      textSoft: mix(fg, p(86, 94), bg),
      textMuted: mix(fg, p(45, 75), bg),
      textFaint: mix(fg, p(28, 52), bg),
    },
    borders: {
      border: mix(fg, p(8, 22), bg),
      borderSoft: mix(fg, p(14, 32), bg),
      divider: mix(fg, p(5, 13), bg),
    },
    semantic: { negative: "#e5484d", warning: "#f5a623", info: "#3b82f6", success: "#30a46c" },
  };
}

export default definePlugin({
  name: "lyra.builtin.custom-theme",
  version: "1.0.0",
  setup({ host }) {
    const register = () => {
      const { customTheme, accent, contrast } = useUiStore.getState();
      const spec = deriveCustomSpec(customTheme, accent, contrast);
      host.extensions.contribute(THEME, {
        id: CUSTOM_THEME_ID,
        label: spec.label,
        scheme: spec.scheme,
        icon: "spark",
        order: 99, // after the built-in packs, before plugin themes
        tokens: buildTokenMap(spec),
      });
    };

    register();
    // Re-derive when the base colors, shared accent, or global contrast
    // change. applyTheme then re-applies the tokens.
    const unsub = useUiStore.subscribe((s, p) => {
      if (s.customTheme !== p.customTheme || s.accent !== p.accent || s.contrast !== p.contrast)
        register();
    });
    disposeOnHmr(unsub);
  },
});
