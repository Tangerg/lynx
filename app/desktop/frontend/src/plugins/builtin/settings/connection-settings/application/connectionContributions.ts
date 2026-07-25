import { CONNECTION_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function connectionSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: CONNECTION_PANE,
    label: "settings.pane.connection",
    group: "general",
    icon: "globe",
    order: 5,
    component,
  };
}
