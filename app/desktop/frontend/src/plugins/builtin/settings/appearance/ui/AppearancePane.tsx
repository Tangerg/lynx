// Composer for the appearance settings pane. The component itself
// only assembles the sections in order — each section owns its own
// state subscriptions, so AppearancePane stays free of store wiring.
//
// Order: theme → accent → contrast → font → shape/motion → language.
// Palette-defining choices first (theme + its two global modifiers),
// then typography, then shape/motion mechanics, with language last.

import { AccentSection } from "./AccentSection";
import { ContrastSection } from "./ContrastSection";
import { CustomThemeColors } from "./CustomThemeColors";
import { FontSection } from "./FontSection";
import { LanguageSection } from "./LanguageSection";
import { ShapeMotionSection } from "./ShapeMotionSection";
import { ThemeSection } from "./ThemeSection";

export function AppearancePane() {
  return (
    <div>
      <ThemeSection />
      {/* Only renders when the "Custom" theme is active. */}
      <CustomThemeColors />
      <AccentSection />
      <ContrastSection />
      <FontSection />
      <ShapeMotionSection />
      <LanguageSection />
    </div>
  );
}
