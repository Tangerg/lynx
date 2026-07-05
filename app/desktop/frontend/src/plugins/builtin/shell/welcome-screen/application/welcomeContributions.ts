import type { LayoutSlotSpec } from "@/plugins/sdk";

export function welcomeEmptySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "welcome",
    order: 0,
    component,
  };
}
