import type { LayoutSlotSpec } from "@/plugins/sdk";

export function notificationsStatusSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "notifications",
    order: 10,
    component,
  };
}
