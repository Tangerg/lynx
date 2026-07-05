import type { LayoutSlotSpec } from "@/plugins/sdk";

export function messageCopyActionSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "copy",
    order: 0,
    component,
  };
}

export function messageEditActionSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "edit",
    order: 5,
    component,
  };
}

export function messageRegenerateActionSlot(
  component: LayoutSlotSpec["component"],
): LayoutSlotSpec {
  return {
    id: "regenerate",
    order: 10,
    component,
  };
}

export function messageFeedbackActionSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "feedback",
    order: 15,
    component,
  };
}
