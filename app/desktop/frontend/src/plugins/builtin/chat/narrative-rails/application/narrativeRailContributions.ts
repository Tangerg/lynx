import type { LayoutSlotSpec } from "@/plugins/sdk";

export function turnRailSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return { id: "turn-rail", order: 0, component };
}
