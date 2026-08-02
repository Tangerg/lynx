import { CompletionSoundSection, StreamRevealSection } from "./PrefSections";
import { SettingsGroup } from "../../public";

export function PersonalizationPane() {
  return (
    <SettingsGroup>
      <StreamRevealSection />
      <CompletionSoundSection />
    </SettingsGroup>
  );
}
