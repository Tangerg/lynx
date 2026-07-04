import type { LayoutSlotSpec, SettingsPaneSpec } from "@/plugins/sdk";

export function shortcutsProviderSlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "shortcuts-provider",
    // Render before toaster (which is order 100) so the side-effect mount is
    // in place before any visible overlays.
    order: 50,
    component,
  };
}

export function shortcutsSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "shortcuts",
    label: "Shortcuts",
    icon: "command",
    order: 50,
    component,
  };
}
