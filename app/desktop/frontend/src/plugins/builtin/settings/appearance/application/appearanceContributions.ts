import type { SettingsPaneSpec } from "@/plugins/sdk";

export function appearanceSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "appearance",
    label: "settings.pane.appearance",
    group: "general",
    icon: "spark",
    order: 0,
    component,
  };
}
