import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installPersonalizationPreferencesPort } from "./adapters/uiPersonalizationPreferences";
import { personalizationSettingsPane } from "./application/personalizationContributions";
import { PersonalizationPane } from "./ui/PersonalizationPane";

export default definePlugin({
  name: "lyra.builtin.personalization",
  version: "1.0.0",
  setup({ host }) {
    const disposePreferences = installPersonalizationPreferencesPort();
    registerSettingsPane(host, personalizationSettingsPane(PersonalizationPane));
    return disposePreferences;
  },
});
