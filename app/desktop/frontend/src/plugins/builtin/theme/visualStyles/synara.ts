import type { VisualStyleSpec } from "@/plugins/sdk";
import { SYNARA_MOTION, SYNARA_TOKENS } from "./tokens";

export const synaraStyle: VisualStyleSpec = {
  id: "synara",
  label: "Synara",
  description: "Floating workspace card, frosted navigation and quiet native controls.",
  order: 0,
  traits: { regions: "floating-card", controls: "quiet" },
  motion: SYNARA_MOTION,
  preview: {
    canvas: "#fbfbfc",
    sidebar: "#eef0f4",
    dock: "#ffffff",
    edge: "rgb(15 23 42 / 0.12)",
    accent: "#6d5dfc",
  },
  tokens: SYNARA_TOKENS,
};
