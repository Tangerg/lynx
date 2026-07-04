import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installPersonalizationPreferencesPort } from "./adapters/uiPersonalizationPreferences";
import { PersonalizationPane } from "./ui/PersonalizationPane";

export default definePlugin({
  name: "lyra.builtin.personalization",
  version: "1.0.0",
  setup({ host }) {
    installPersonalizationPreferencesPort();
    registerSettingsPane(host, {
      id: "personalization",
      label: "settings.pane.personalization",
      group: "general",
      icon: "user",
      order: 1,
      component: PersonalizationPane,
    });
  },
});
