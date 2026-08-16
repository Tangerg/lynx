import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { installPersonalizationPreferencesPort } from "./adapters/uiPersonalizationPreferences";
import { personalizationSettingsPane } from "./application/personalizationContributions";

const PersonalizationPane = lazy(() =>
  import("./ui/PersonalizationPane").then(({ PersonalizationPane }) => ({
    default: PersonalizationPane,
  })),
);

export default definePlugin({
  name: "lyra.builtin.personalization",
  setup(ctx) {
    const disposePreferences = installPersonalizationPreferencesPort();
    registerSettingsPane(ctx, personalizationSettingsPane(PersonalizationPane));
    ctx.cleanup(disposePreferences);
  },
});
