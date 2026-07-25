// Composer for the appearance settings pane. The component itself only lays out
// sections; each section owns its own preference subscription.

import { Surface } from "@/ui";
import { useT } from "@/lib/i18n";
import { AccentSection } from "./AccentSection";
import { ContrastSection } from "./ContrastSection";
import { CustomThemeColors } from "./CustomThemeColors";
import { FontSection } from "./FontSection";
import { LanguageSection } from "./LanguageSection";
import { ShapeMotionSection } from "./ShapeMotionSection";
import { ThemeSection } from "./ThemeSection";

export function AppearancePane() {
  const t = useT();
  return (
    <div className="pb-16">
      <header>
        <h1 className="m-0 text-display-lg font-semibold text-fg">
          {t("settings.pane.appearance")}
        </h1>
        <p className="m-0 mt-2 text-ui-lg leading-6 text-fg-muted">
          {t("settings.appearance.hero")}
        </p>
      </header>
      <Surface className="mt-8 overflow-hidden">
        <ThemeSection />
        <CustomThemeColors />
        <AccentSection />
        <ContrastSection />
        <FontSection />
        <ShapeMotionSection />
        <LanguageSection />
      </Surface>
    </div>
  );
}
