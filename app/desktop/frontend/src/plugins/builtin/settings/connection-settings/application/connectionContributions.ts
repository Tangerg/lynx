import type { SettingsPaneSpec } from "@/plugins/sdk";

export function connectionSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "connection",
    label: "settings.pane.connection",
    group: "general",
    icon: "globe",
    order: 5,
    component,
  };
}
