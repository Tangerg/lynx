// Custom-theme color editor — the background + foreground the "Custom" theme
// derives its full palette from (accent is edited in AccentSection). Only shown
// when the Custom theme is active; edits apply live (custom-theme plugin
// re-derives + re-registers, uiStore re-applies).

import { useT } from "@/lib/i18n";
import { useCustomThemePreference } from "../application/appearancePreferences";
import { SettingRow } from "../../public";

function ColorRow({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (hex: string) => void;
}) {
  return (
    <label className="flex items-center justify-between gap-3 rounded-md bg-surface-2 px-3 py-1.5 transition-colors hover:bg-surface-3">
      <span className="text-[13px] text-fg-muted">{label}</span>
      <span className="relative inline-flex items-center gap-2">
        <span className="font-mono text-[12px] uppercase text-fg">{value}</span>
        <span
          className="h-4.5 w-4.5 rounded-full border-[0.5px] border-field bg-clip-padding"
          style={{ background: value }}
        />
        {/* Hidden native picker — clicking the row opens the OS color wheel. */}
        <input
          type="color"
          aria-label={label}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="absolute inset-0 h-full w-full opacity-0"
        />
      </span>
    </label>
  );
}

export function CustomThemeColors() {
  const t = useT();
  const { theme, customTheme, setCustomTheme } = useCustomThemePreference();

  // Only relevant while the Custom theme is the active one.
  if (theme !== "custom") return null;

  return (
    <SettingRow
      label={t("settings.customColors")}
      sub={t("settings.customColors.sub")}
      align="start"
    >
      <div className="grid max-w-[300px] gap-2">
        <ColorRow
          label={t("settings.color.bg")}
          value={customTheme.bg}
          onChange={(bg) => setCustomTheme({ bg })}
        />
        <ColorRow
          label={t("settings.color.fg")}
          value={customTheme.fg}
          onChange={(fg) => setCustomTheme({ fg })}
        />
      </div>
    </SettingRow>
  );
}
