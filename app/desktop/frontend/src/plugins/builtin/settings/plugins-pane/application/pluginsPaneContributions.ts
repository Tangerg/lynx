import { PLUGINS_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function pluginsSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: PLUGINS_PANE,
    label: "settings.pane.plugins",
    group: "integrations",
    icon: "tool",
    order: 99,
    component,
  };
}
