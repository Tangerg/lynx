// Composer for the appearance settings pane. The component itself
// only assembles the 5 sections in order — each section owns its own
// state subscriptions, so AppearancePane stays free of store wiring.

import { AccentSection } from "./AccentSection";
import { FontSection } from "./FontSection";
import { LanguageSection, MessageStyleSection } from "./PrefSections";
import { ThemeSection } from "./ThemeSection";

export function AppearancePane() {
  return (
    <div>
      <ThemeSection />
      <AccentSection />
      <FontSection />
      <MessageStyleSection />
      <LanguageSection />
    </div>
  );
}
