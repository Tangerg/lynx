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

/** Before the composer's send control: Goal mode changes how the draft is
 * executed, while Send remains the terminal action on the right edge. */
export function goalLauncherSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "goal-launcher",
    order: 90,
    component,
  };
}
