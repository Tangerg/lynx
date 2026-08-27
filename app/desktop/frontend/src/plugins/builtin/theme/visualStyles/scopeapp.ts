import type { VisualStyleSpec } from "@/plugins/sdk";
import { visualStyleTokens, WORKBENCH_MOTION } from "./tokens";

/**
 * The product's one built-in workbench language.
 *
 * JetBrains tool windows: a quiet reading plane framed by opaque chrome columns,
 * separated by value and a half-pixel hairline at each seam, with borderless
 * cards and technical type set in mono. Colour stays semantic, so every colour
 * theme inherits the same region algorithm without hard-coded light or dark
 * values — the style says where the depth is, the theme says how dark.
 */
export const scopeappStyle: VisualStyleSpec = {
  id: "scopeapp",
  label: "Scope Workbench",
  description: "Tool-window geometry: opaque columns, borderless cards, half-pixel seams.",
  order: -10,
  traits: { regions: "tool-windows", controls: "quiet" },
  motion: WORKBENCH_MOTION,
  preview: {
    canvas: "#1d1f23",
    sidebar: "#2a2d32",
    dock: "#24272c",
    edge: "#3a3d42",
    accent: "#3574f0",
  },
  tokens: visualStyleTokens({}),
};
