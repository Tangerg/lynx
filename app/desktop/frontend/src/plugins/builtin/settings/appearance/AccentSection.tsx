// Accent picker — one swatch per registered AccentSpec plus a custom
// color slot that opens the OS-native color wheel.

import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { ACCENT, resolveScheme, useExtensionPoint } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";
import { SettingRow } from "../SettingRow";

// Conic gradient used when no custom color is active — communicates
// "click me, you can pick anything" without committing to a default hue.
const RAINBOW_HINT =
  "conic-gradient(from 0deg, #ef4444, #f59e0b, #eab308, #22c55e, #06b6d4, #6366f1, #a855f7, #ec4899, #ef4444)";

function CustomAccentPicker({
  value,
  isActive,
  onChange,
  label,
}: {
  value: string;
  isActive: boolean;
  onChange: (hex: string) => void;
  label: string;
}) {
  return (
    <label
      title={label}
      aria-label={label}
      className={cn(
        "relative inline-grid h-4.5 w-4.5 place-items-center rounded-full border-2 border-transparent bg-clip-padding transition-[transform,box-shadow] duration-150 active:scale-95",
        isActive && "border-surface shadow-[0_0_0_1.5px_var(--color-text)]",
      )}
      style={{ background: isActive ? value : RAINBOW_HINT }}
    >
      {/* Hidden native input — the visible swatch is the label; click
          opens the OS color picker. */}
      <input
        type="color"
        aria-label="Custom accent color"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="absolute inset-0 h-full w-full opacity-0"
      />
    </label>
  );
}

export function AccentSection() {
  const t = useT();
  const accents = useExtensionPoint(ACCENT);
  const accent = useUiStore((s) => s.accent);
  const setAccent = useUiStore((s) => s.setAccent);
  // Swatch must paint the color that ACTUALLY applies in the current scheme:
  // presets carry a hand-tuned `light` variant applyTheme uses in light themes,
  // so painting the dark hex would show a color different from the one the app
  // renders. (The stored key stays the dark hex — the active check is unchanged.)
  const light = resolveScheme(useUiStore((s) => s.theme)) === "light";

  const isCustom = !accents.some((a) => a.dark === accent);

  return (
    <SettingRow label={t("settings.accent")} sub={t("settings.accent.sub")}>
      <div className="flex flex-wrap gap-2.5 justify-start items-center">
        {accents.map((a) => (
          <button
            key={a.id}
            type="button"
            onClick={() => setAccent(a.dark)}
            title={`${t("settings.accent")}: ${a.label}`}
            aria-label={`${t("settings.accent")}: ${a.label}`}
            aria-pressed={accent === a.dark}
            style={{ background: light ? (a.light ?? a.dark) : a.dark }}
            className={cn(
              "h-4.5 w-4.5 rounded-full border-2 border-transparent bg-clip-padding p-0 transition-[transform,box-shadow] duration-150 active:scale-95",
              accent === a.dark && "border-surface shadow-[0_0_0_1.5px_var(--color-text)]",
            )}
          />
        ))}
        <CustomAccentPicker
          value={accent}
          isActive={isCustom}
          onChange={setAccent}
          label={t("settings.accent.custom")}
        />
      </div>
    </SettingRow>
  );
}
