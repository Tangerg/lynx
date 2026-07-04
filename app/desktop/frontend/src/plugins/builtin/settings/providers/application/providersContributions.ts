import type { SettingsPaneSpec } from "@/plugins/sdk";

export function providersSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "providers",
    label: "settings.pane.providers",
    group: "models",
    icon: "spark",
    order: 50,
    component,
  };
}
