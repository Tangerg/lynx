import type { SettingsPaneSpec } from "@/plugins/sdk";

export function approvalsSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: "approvals",
    label: "settings.pane.approvals",
    group: "agent",
    icon: "shield",
    order: 55,
    component,
  };
}
