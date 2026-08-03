import type { LayoutSlotSpec } from "@/plugins/sdk";

/** The empty-home slot. Ordered first because when this renders at all, nothing else
 *  on that screen is actionable. */
export function providerSetupEmptySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "provider-setup",
    order: 0,
    component,
  };
}
