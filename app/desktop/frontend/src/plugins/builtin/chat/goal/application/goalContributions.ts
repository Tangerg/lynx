import type { LayoutSlotSpec } from "@/plugins/sdk";

// Goal mode's banner sits just below the plan-progress banner (order 0) in the
// chat.banner.top slot — both are run-scoped status strips above the composer.
export function goalBannerSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "goal",
    order: 1,
    component,
  };
}
