import { CompletionSoundSection, MessageStyleSection, StreamRevealSection } from "./PrefSections";

export function PersonalizationPane() {
  return (
    <div>
      <MessageStyleSection />
      <StreamRevealSection />
      <CompletionSoundSection />
    </div>
  );
}
