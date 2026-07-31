import { CompletionSoundSection, MessageStyleSection, StreamRevealSection } from "./PrefSections";
import { SettingsGroup } from "../../public";

export function PersonalizationPane() {
  return (
    <SettingsGroup>
      <MessageStyleSection />
      <StreamRevealSection />
      <CompletionSoundSection />
    </SettingsGroup>
  );
}
