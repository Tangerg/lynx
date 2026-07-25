import { APPEARANCE_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function appearanceSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: APPEARANCE_PANE,
    label: "settings.pane.appearance",
    group: "general",
    icon: "spark",
    order: 0,
    component,
  };
}
