import type { Contributor, SettingsPaneSpec } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk";

export function registerSettingsPane(ctx: Contributor, pane: SettingsPaneSpec) {
  return ctx.contribute(SETTINGS_PANE, pane);
}
