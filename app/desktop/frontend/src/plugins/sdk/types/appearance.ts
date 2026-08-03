import type { Scheme, VisualStyleMotion } from "@/lib/appearance";

/** A swappable colour palette. Geometry and component treatment belong to a visual style. */
export interface ColorThemeSpec {
  /** Stable id persisted by the UI preference store. */
  id: string;
  label: string;
  scheme: Scheme;
  icon?: string;
  order?: number;
  /** CSS custom properties without the leading `--`. */
  tokens?: Record<string, string>;
  /**
   * Opt in to having this theme's neutral family follow the LIVE accent.
   *
   * Each step says where a neutral sits — its OKLCH lightness, and the chroma it
   * carries at the reference accent. The shell rewrites them onto whatever accent the
   * user picked; the literals in `tokens` remain the family at the default accent, and
   * are what a cold boot paints.
   *
   * A palette theme must leave this undefined: Solarized's base3 is Solarized, not a
   * tint of whatever accent happens to be selected.
   */
  neutralSteps?: ThemeNeutralSteps;
}

/** One neutral's place: OKLCH lightness (0-100) and chroma at the reference accent. */
export interface NeutralStep {
  l: number;
  c: number;
}

export interface ThemeNeutralSteps {
  surface: NeutralStep;
  elevated: NeutralStep;
  sunken: NeutralStep;
  border: NeutralStep;
  borderSoft: NeutralStep;
}

export interface AccentSpec {
  id: string;
  label: string;
  dark: string;
  light?: string;
  order?: number;
}

export type RegionLayout = "floating-card" | "flush-panes" | "tonal-columns" | "tool-windows";
export type ControlTreatment = "quiet" | "outlined" | "tonal" | "elevated";

export interface VisualStylePreview {
  canvas: string;
  sidebar: string;
  dock: string;
  edge: string;
  accent: string;
}

/**
 * A complete component and region design language, independent from colour.
 *
 * `traits` expose structural intent to shell CSS through data attributes. Tokens
 * own the metrics and materials consumed by the shell and shared atoms. Keeping
 * both in one contribution lets a third-party style change pane relationships,
 * not merely repaint existing controls.
 */
export interface VisualStyleSpec {
  id: string;
  label: string;
  description: string;
  order?: number;
  traits: {
    regions: RegionLayout;
    controls: ControlTreatment;
  };
  motion: VisualStyleMotion;
  preview: VisualStylePreview;
  /** CSS custom properties without the leading `--`. */
  tokens: Record<string, string>;
}
