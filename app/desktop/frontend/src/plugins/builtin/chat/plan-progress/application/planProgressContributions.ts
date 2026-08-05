import type { LayoutSlotSpec } from "@/plugins/sdk";

/** Below the goal banner: the goal is the standing order, this is how the agent
 *  proposes to pursue it. */
export function planProgressBannerSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "plan-progress",
    order: 1,
    component,
  };
}
