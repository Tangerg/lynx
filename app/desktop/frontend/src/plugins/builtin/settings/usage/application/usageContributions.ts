import { USAGE_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function usageSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: USAGE_PANE,
    label: "settings.pane.usage",
    group: "models",
    icon: "chart",
    order: 55,
    component,
  };
}
