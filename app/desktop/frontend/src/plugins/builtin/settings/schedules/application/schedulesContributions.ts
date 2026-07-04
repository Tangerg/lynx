import type { SettingsPaneSpec } from "@/plugins/sdk";

export function schedulesSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "schedules",
    label: "settings.pane.schedules",
    group: "agent",
    icon: "command",
    order: 58,
    component,
  };
}
