import type { VisualStyleSpec } from "@/plugins/sdk";
import { visualStyleTokens, WORKBENCH_MOTION } from "./tokens";

/**
 * The product's one built-in workbench language.
 *
 * Nova supplies the humane density, rounded selection states and clear task
 * grouping. JetBrains Classic supplies the geometry: flush tool windows, one
 * hairline per boundary, and a quiet centre reading plane between two chrome
 * columns. Colour stays semantic so every colour theme inherits the same region
 * algorithm without hard-coded light or dark values.
 */
export const lyraStyle: VisualStyleSpec = {
  id: "lyra",
  label: "Lynx Workbench",
  description: "Nova navigation and interaction rhythm on JetBrains-style tool-window geometry.",
  order: -10,
  traits: { regions: "tool-windows", controls: "outlined" },
  motion: WORKBENCH_MOTION,
  preview: {
    canvas: "#ffffff",
    sidebar: "#f2f3f5",
    dock: "#f6f7f8",
    edge: "#d8dadd",
    accent: "#3574f0",
  },
  tokens: visualStyleTokens({}),
};
