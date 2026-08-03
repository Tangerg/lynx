import type { LayoutSlotSpec } from "@/plugins/sdk";

export function windowControlsSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "window-controls",
    order: 0,
    component,
  };
}
