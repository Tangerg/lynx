// Global contrast slider — drives `--depth-step`, the color-mix amount
// every surface ladder (`--color-surface-2/3/4`) derives from, so it
// affects all themes, not just Custom. 0 = flat, 100 = maximum surface
// separation.

import { Slider } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useUiStore } from "@/state/uiStore";
import { SettingRow } from "./SettingRow";

export function ContrastSection() {
  const t = useT();
  const contrast = useUiStore((s) => s.contrast);
  const setContrast = useUiStore((s) => s.setContrast);

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
        <span className="w-7 text-right font-mono text-[12px] tabular-nums text-fg-muted">
          {contrast}
        </span>
      </div>
    </SettingRow>
  );
}
