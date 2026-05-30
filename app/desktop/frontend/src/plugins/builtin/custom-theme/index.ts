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
import { useUiStore } from "@/state/uiStore";
import { buildTokenMap } from "../themes/tokens";
import type { ThemePluginSpec } from "../themes/types";
import type { CustomTheme } from "@/state/uiStore";

const CUSTOM_THEME_ID = "custom";

// mix(a, pct, b) → CSS color-mix string: pct% of `a`, rest `b`. Resolved by
// the browser, so the derived ladder tracks the base colors exactly.
const mix = (a: string, pct: number, b: string): string => `color-mix(in srgb, ${a} ${pct}%, ${b})`;

/** Derive a full theme spec from the custom bg/fg + the shared global accent. */
function deriveCustomSpec(ct: CustomTheme, accent: string): ThemePluginSpec {
  const { bg, fg } = ct;
  const scheme: "dark" | "light" = colord(bg).isDark() ? "dark" : "light";
  const extreme = scheme === "dark" ? "#ffffff" : "#000000";
  return {
    id: CUSTOM_THEME_ID,
    label: "Custom",
    scheme,
    brand: { accent, textOnAccent: colord(accent).isDark() ? "#ffffff" : "#000000" },
    surfaces: { bg, surface: mix(fg, 6, bg) },
    ink: {
      text: fg,
      textBright: mix(fg, 80, extreme), // nudge toward pure white/black
      textSoft: mix(fg, 88, bg),
      textMuted: mix(fg, 60, bg),
      textFaint: mix(fg, 42, bg),
    },
    borders: {
      border: mix(fg, 14, bg),
      borderSoft: mix(fg, 22, bg),
      divider: mix(fg, 9, bg),
      appDivider: bg,
    },
    semantic: { negative: "#e5484d", warning: "#f5a623", info: "#3b82f6", success: "#30a46c" },
  };
}

export default definePlugin({
  name: "lyra.builtin.custom-theme",
  version: "1.0.0",
  setup({ host }) {
    const register = () => {
      const { customTheme, accent } = useUiStore.getState();
      const spec = deriveCustomSpec(customTheme, accent);
      host.theme.registerTheme({
        id: CUSTOM_THEME_ID,
        label: spec.label,
        scheme: spec.scheme,
        icon: "spark",
        order: 99, // after the built-in packs, before plugin themes
        tokens: buildTokenMap(spec),
      });
    };

    register();
    // Re-derive when the base colors OR the shared accent change (the latter
    // flips textOnAccent black/white). applyTheme then re-applies the tokens.
    const unsub = useUiStore.subscribe((s, p) => {
      if (s.customTheme !== p.customTheme || s.accent !== p.accent) register();
    });
    disposeOnHmr(unsub);
  },
});
