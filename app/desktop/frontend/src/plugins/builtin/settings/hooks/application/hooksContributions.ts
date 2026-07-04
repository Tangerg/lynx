import type { SettingsPaneSpec } from "@/plugins/sdk";

export function hooksSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "hooks",
    label: "settings.pane.hooks",
    group: "agent",
    icon: "lightning",
    order: 57,
    component,
  };
}
