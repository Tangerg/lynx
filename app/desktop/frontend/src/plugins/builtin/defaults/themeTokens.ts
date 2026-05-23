// Built-in theme palettes.
//
// Each map is `<css-var-name-without-leading-dashes>` → value, written to
// `:root.style` by `applyTheme()` when the corresponding theme is active.
// The values here are the source of truth — the equivalent declarations
// in `styles/tokens.css` (`:root`) and `styles/theme.css` (`.theme-light`)
// are fallbacks that cover the brief window between first paint and the
// plugin registering.
//
// Only theme-dependent tokens live here. Spacing, type, motion, radius —
// none of those vary across themes, so they stay declared once in
// tokens.css and are referenced from these palettes via `var(--…)` when
// composing derived values (none currently — kept literal for clarity).

/** Dark palette — the system default. */
export const darkTokens: Record<string, string> = {
  // Brand
  "color-accent":         "#1ed760",
  "color-accent-border":  "#1db954",
  "color-accent-press":   "#169c46",

  // Surface ladder — canvas + surface (L1) are manual anchors; L2/L3/L4
  // derive via color-mix() in tokens.css from --color-text + --depth-step
  // so the formula does the work and we don't duplicate hexes here.
  "color-bg":             "#010102",
  "color-surface":        "#181a1d",
  "depth-step":           "5%",

  // Ink
  "color-text":           "#f7f8f8",
  "color-text-bright":    "#ffffff",
  "color-text-soft":      "#d0d6e0",
  "color-text-muted":     "#8a8f98",
  "color-text-faint":     "#62666d",
  "color-text-on-accent": "#000000",

  // Hairlines
  "color-border":         "#23252a",
  "color-border-soft":    "#34343a",
  "color-divider":        "#3e3e44",
  "color-app-divider":    "#23252a",

  // Semantic
  "color-negative":       "#ee0000",
  "color-warning":        "#f5a623",
  "color-info":           "#0070f3",
  "color-success":        "#27a644",

  // CTA — dark mode lets accent drive the primary action
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // Shadows — dark depth comes from surface ladder, not shadow.
  // The floating-overlay layer (palette, lightbox) still wants a shadow,
  // exposed as --shadow-lg.
  "shadow-xs":            "none",
  "shadow-sm":            "none",
  "shadow-md":            "none",
  "shadow-lg":
    "inset 0 1px 0 rgba(255, 255, 255, 0.04), " +
    "0 1px 2px rgba(0, 0, 0, 0.40), " +
    "0 8px 16px -4px rgba(0, 0, 0, 0.50), " +
    "0 24px 32px -8px rgba(0, 0, 0, 0.60), " +
    "inset 0 0 0 1px var(--color-border)",
  "shadow-card":          "none",
  "shadow-dialog":        "var(--shadow-lg)",
  "shadow-soft":          "none",
  "shadow-pop":           "var(--shadow-lg)",
  "shadow-glow":          "0 0 12px color-mix(in srgb, var(--color-accent) 50%, transparent)",
  "shadow-input-focus":
    "0 0 0 2px color-mix(in srgb, var(--color-accent) 30%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

/** Light palette — Vercel dashboard inspired. */
export const lightTokens: Record<string, string> = {
  // Brand — dimmer green on white (#15883e reads better than full #1ed760)
  "color-accent":         "#15883e",
  "color-accent-border":  "#117134",
  "color-accent-press":   "#0c5d2a",

  // Surface ladder
  "color-bg":             "#fafafa",
  "color-surface":        "#ffffff",
  "depth-step":           "5%",

  // Ink — Vercel #171717 / #4d4d4d / #6f6f6f ladder
  "color-text":           "#171717",
  "color-text-bright":    "#000000",
  "color-text-soft":      "#4d4d4d",
  "color-text-muted":     "#6f6f6f",
  "color-text-faint":     "#a1a1a1",
  "color-text-on-accent": "#ffffff",

  // Hairlines — Vercel #ebebeb / #d4d4d6 / #a1a1a1
  "color-border":         "#ebebeb",
  "color-border-soft":    "#d4d4d6",
  "color-divider":        "#a1a1a1",
  "color-app-divider":    "#ebebeb",

  // Semantic
  "color-negative":       "#ee0000",
  "color-warning":        "#f5a623",
  "color-info":           "#0070f3",
  "color-success":        "#15883e",

  // CTA — Vercel signature: black-on-white, decoupled from accent so
  // accent stays reserved for "live / running" indicators.
  "color-cta":            "#000000",
  "color-cta-hover":      "#222222",
  "color-cta-text":       "#ffffff",

  // Shadows — light surfaces NEED shadows to read as elevated
  "shadow-xs":            "0 1px 2px rgba(15, 15, 15, 0.04)",
  "shadow-sm":
    "0 1px 2px rgba(15, 15, 15, 0.04), " +
    "0 2px 6px rgba(15, 15, 15, 0.06)",
  "shadow-md":
    "0 2px 4px rgba(15, 15, 15, 0.04), " +
    "0 8px 20px rgba(15, 15, 15, 0.10)",
  "shadow-lg":
    "0 4px 12px rgba(15, 15, 15, 0.08), " +
    "0 24px 60px -12px rgba(15, 15, 15, 0.18)",
  "shadow-card":          "var(--shadow-sm)",
  "shadow-dialog":        "var(--shadow-lg)",
  "shadow-pop":           "var(--shadow-lg)",
  "shadow-soft":          "var(--shadow-xs)",
  "shadow-glow":          "0 0 12px color-mix(in srgb, var(--color-accent) 40%, transparent)",
  "shadow-input-focus":
    "0 0 0 3px color-mix(in srgb, var(--color-accent) 14%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};
