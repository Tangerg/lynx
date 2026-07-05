import type { LayoutSlotSpec } from "@/plugins/sdk";

export function sidebarFooterSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "user-card",
    order: 0,
    component,
  };
}
