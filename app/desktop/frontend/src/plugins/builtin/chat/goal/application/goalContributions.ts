import type { LayoutSlotSpec } from "@/plugins/sdk";

/** Above the plan banner: the goal is the standing order, the plan is how the
 *  agent proposes to pursue it. */
export function goalBannerSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "goal",
    order: 0,
    component,
  };
}
