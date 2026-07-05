import type { LayoutSlotSpec } from "@/plugins/sdk";

export function planProgressBannerSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "plan-progress",
    order: 0,
    component,
  };
}
