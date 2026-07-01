// Built-in plugin: the Personalization settings pane. Home for "how the app
// behaves/feels for me" preferences that aren't theme/typography — starting
// with the message bubble style (moved out of Appearance). Future
// personalization knobs land here too.

import { CompletionSoundSection, MessageStyleSection, StreamRevealSection } from "./PrefSections";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { installPersonalizationPreferencesPort } from "./adapters/uiPersonalizationPreferences";

function PersonalizationPane() {
  return (
    <div>
      <MessageStyleSection />
      <StreamRevealSection />
      <CompletionSoundSection />
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.personalization",
  version: "1.0.0",
  setup({ host }) {
    installPersonalizationPreferencesPort();
    host.extensions.contribute(SETTINGS_PANE, {
      id: "personalization",
      label: "settings.pane.personalization",
      group: "general",
      icon: "user",
      order: 1, // right after Appearance (0)
      component: PersonalizationPane,
    });
  },
});
