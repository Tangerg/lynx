import type { LayoutSlotSpec } from "@/plugins/sdk";

export function toasterOverlaySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "toaster",
    // Render last in the overlay slot so toast portals stay above command UI.
    order: 100,
    component,
  };
}
