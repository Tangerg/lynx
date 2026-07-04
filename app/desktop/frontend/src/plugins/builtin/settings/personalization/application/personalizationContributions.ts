import type { SettingsPaneSpec } from "@/plugins/sdk";

export function personalizationSettingsPane(
  component: SettingsPaneSpec["component"],
): SettingsPaneSpec {
  return {
    id: "personalization",
    label: "settings.pane.personalization",
    group: "general",
    icon: "user",
    order: 1,
    component,
  };
}
