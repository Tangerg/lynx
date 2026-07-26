import { SCHEDULES_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function schedulesSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: SCHEDULES_PANE,
    label: "settings.pane.schedules",
    group: "agent",
    icon: "clock",
    order: 58,
    component,
  };
}
