import type { ContributingHost, SettingsPaneSpec } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk";

export function registerSettingsPane(host: ContributingHost, pane: SettingsPaneSpec) {
  return host.extensions.contribute(SETTINGS_PANE, pane);
}
