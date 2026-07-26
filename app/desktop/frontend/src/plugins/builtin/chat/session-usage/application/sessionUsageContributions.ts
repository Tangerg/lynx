import type { LayoutSlotSpec } from "@/plugins/sdk";

export function sessionUsageStatusSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "session-usage",
    order: 10,
    component,
  };
}
