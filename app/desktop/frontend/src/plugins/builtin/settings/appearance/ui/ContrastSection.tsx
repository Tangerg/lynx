// Global contrast slider — drives `--depth-step`, the color-mix amount
// every surface ladder (`--color-surface-2/3/4`) derives from, so it
// affects all themes, not just Custom. 0 = flat, 100 = maximum surface
// separation.

import { Slider } from "@/ui";
import { useT } from "@/lib/i18n";
import { useContrastPreference } from "../application/appearancePreferences";
import { SettingRow } from "../../public";

export function ContrastSection() {
  const t = useT();
  const { contrast, setContrast } = useContrastPreference();

  return (
    <SettingRow label={t("settings.contrast")} sub={t("settings.contrast.sub")} align="start">
      <div className="flex items-center gap-3">
        <Slider
          value={contrast}
          min={0}
          max={100}
          onValueChange={setContrast}
          ariaLabel={t("settings.contrast")}
        />
        <span className="w-7 text-right font-mono text-[12px] text-fg-muted">{contrast}</span>
      </div>
    </SettingRow>
  );
}
