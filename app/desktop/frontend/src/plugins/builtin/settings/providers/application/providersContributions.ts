import { PROVIDERS_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function providersSettingsPane(component: SettingsPaneSpec["component"]): SettingsPaneSpec {
  return {
    id: PROVIDERS_PANE,
    label: "settings.pane.providers",
    group: "models",
    icon: "spark",
    order: 50,
    component,
  };
}
