import type { LayoutSlotSpec } from "@/plugins/sdk";

export function sessionUsageBannerSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "session-usage",
    order: 10,
    component,
  };
}
