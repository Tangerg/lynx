import type { SettingsPaneSpec } from "@/plugins/sdk";

export function pluginsSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "plugins",
    label: "settings.pane.plugins",
    group: "integrations",
    icon: "tool",
    order: 99,
    component,
  };
}
