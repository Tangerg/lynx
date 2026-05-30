// Composer for the appearance settings pane. The component itself
// only assembles the 6 sections in order — each section owns its own
// state subscriptions, so AppearancePane stays free of store wiring.

import { AccentSection } from "./AccentSection";
import { CustomThemeColors } from "./CustomThemeColors";
import { FontSection } from "./FontSection";
import { LanguageSection } from "./PrefSections";
import { ShapeMotionSection } from "./ShapeMotionSection";
import { ThemeSection } from "./ThemeSection";

export function AppearancePane() {
  return (
    <div>
      <ThemeSection />
      {/* Only renders when the "Custom" theme is active. */}
      <CustomThemeColors />
      <AccentSection />
      <FontSection />
      <ShapeMotionSection />
      <LanguageSection />
    </div>
  );
}
