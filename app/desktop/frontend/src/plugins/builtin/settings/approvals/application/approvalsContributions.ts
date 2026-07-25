import { APPROVALS_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function approvalsSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: APPROVALS_PANE,
    label: "settings.pane.approvals",
    group: "agent",
    icon: "shield",
    order: 55,
    component,
  };
}
