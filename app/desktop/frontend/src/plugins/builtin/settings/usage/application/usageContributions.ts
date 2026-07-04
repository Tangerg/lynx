import type { SettingsPaneSpec } from "@/plugins/sdk";

export function usageSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "usage",
    label: "settings.pane.usage",
    group: "models",
    icon: "chart",
    order: 55,
    component,
  };
}
