import type { LayoutSlotSpec, WorkspaceViewSpec } from "@/plugins/sdk";

export function kernelChatSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "chat",
    order: 0,
    component,
  };
}

export function kernelSidebarSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "sidebar",
    order: 0,
    component,
  };
}

export function kernelSettingsView(component: WorkspaceViewSpec["component"]): WorkspaceViewSpec {
  return {
    id: "settings",
    title: "settings.title",
    icon: "settings",
    component,
  };
}
