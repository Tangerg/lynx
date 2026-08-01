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
