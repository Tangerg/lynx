import type { LayoutSlotSpec } from "@/plugins/sdk";

export function composerAttachSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "attach",
    order: 0,
    component,
  };
}

export function composerApprovalSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "approval",
    order: 1,
    component,
  };
}

export function composerModelSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "model",
    order: 2,
    component,
  };
}

export function composerSendSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "send",
    order: 100,
    component,
  };
}
