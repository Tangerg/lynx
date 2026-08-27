import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../public";
import { PERSONALIZATION_PANE } from "../public/panes";
import { installPersonalizationPreferencesPort } from "./adapters/uiPersonalizationPreferences";

const PersonalizationPane = lazy(() =>
  import("./ui/PersonalizationPane").then(({ PersonalizationPane }) => ({
    default: PersonalizationPane,
  })),
);

export default definePlugin({
  name: "scopeapp.builtin.personalization",
  setup(ctx) {
    const disposePreferences = installPersonalizationPreferencesPort();
    registerSettingsPane(ctx, {
      id: PERSONALIZATION_PANE,
      label: "settings.pane.personalization",
      group: "general",
      icon: "user",
      order: 1,
      component: PersonalizationPane,
    });
    ctx.cleanup(disposePreferences);
  },
});
