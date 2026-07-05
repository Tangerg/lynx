import type { LayoutSlotSpec } from "@/plugins/sdk";

export function tasksStatusSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "tasks",
    order: 0,
    component,
  };
}
