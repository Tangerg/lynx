// Theme + accent surface — IDE/VS Code-style swappable palettes.

/**
 * A theme — IDE/VS Code-style swappable palette.
 *
 * The theme owns its entire color palette via `tokens`: each entry is a
 * CSS custom property name (WITHOUT the leading `--`) mapped to its value.
 * When a theme becomes active, the host writes every token to
 * `:root.style` (inline) so it overrides whatever the stylesheets declared,
 * and toggles the `theme-{scheme}` class on <html> so structural rules
 * keyed on `.theme-light` / `.theme-dark` still apply.
 *
 * `scheme` is the binary kind (dark vs light) used for:
 *   - The `<html>` class for structural overrides
 *   - Picking the accent variant (light vs dark hex on each accent)
 *   - Asset selection (Shiki / Mermaid theme presets)
 *
 * Themes without `tokens` keep working as metadata-only registrations —
 * the existing stylesheets supply values. Pass `tokens` to take full
 * control of the palette.
 */
export interface ThemeSpec {
  /** Stable id. Persisted by `uiStore` to `lyra.ui`. */
  id: string;
  /** User-facing label. */
  label: string;
  /** Native scheme — drives `<html style="color-scheme">` + accent picker. */
  scheme: "dark" | "light";
  /** Icon name for the segmented control. */
  icon?: string;
  /** Sort hint — lower comes first. */
  order?: number;
  /**
   * CSS custom property values, keyed by name WITHOUT the leading `--`.
   * Applied to `:root.style` when this theme is active.
   * Example: `{ "color-bg": "#010102", "color-surface": "#181a1d" }`
   */
  tokens?: Record<string, string>;
}

/**
 * An accent — a named color with one value per theme scheme. The active
 * scheme's hex is written to `--color-accent`.
 *
 * Defaults: `light` falls back to `dark` when omitted, which is what
 * "monochrome-friendly" accents will want.
 */
export interface ThemeAccentSpec {
  id: string;
  label: string;
  dark: string;
  light?: string;
  /** Sort hint — lower comes first. */
  order?: number;
}
