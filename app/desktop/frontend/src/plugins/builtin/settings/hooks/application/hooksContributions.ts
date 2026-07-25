import { HOOKS_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function hooksSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: HOOKS_PANE,
    label: "settings.pane.hooks",
    group: "agent",
    icon: "lightning",
    order: 57,
    component,
  };
}
