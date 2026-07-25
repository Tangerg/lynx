import { PERSONALIZATION_PANE } from "../../public/panes";
import type { SettingsPaneSpec } from "@/plugins/sdk";

export function personalizationSettingsPane(
  component: SettingsPaneSpec["component"],
): SettingsPaneSpec {
  return {
    id: PERSONALIZATION_PANE,
    label: "settings.pane.personalization",
    group: "general",
    icon: "user",
    order: 1,
    component,
  };
}
