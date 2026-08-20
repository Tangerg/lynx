// Accent picker — one swatch per registered AccentSpec plus a custom
// color slot that opens the OS-native color wheel.

import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { ACCENT, useExtensionPoint } from "@/plugins/sdk";
import { useAccentPreference } from "../application/appearancePreferences";
import { SettingRow } from "../../public";
import { ColorPickerInput, Icon, Pressable } from "@/ui";

// Conic gradient used when no custom color is active — communicates
// "click me, you can pick anything" without committing to a default hue.
const RAINBOW_HINT =
  "conic-gradient(from 0deg, #ef4444, #f59e0b, #eab308, #22c55e, #06b6d4, #6366f1, #a855f7, #ec4899, #ef4444)";

// The colour remains a compact dot, while its target is a full desktop control.
// The old 18px target moved less than half a pixel on press and its selected ring
// was easy to miss, which made a real preference update feel inert.
const SWATCH_TARGET =
  "group/accent relative inline-grid h-7 w-7 place-items-center rounded-full bg-transparent transition-[background-color,transform] duration-[var(--dur-fast)] hover:bg-hover active:scale-[var(--press-scale)]";
const SWATCH_CHROME =
  "h-5 w-5 rounded-full border-2 border-transparent bg-clip-padding transition-[transform,box-shadow] duration-[var(--dur-fast)] group-hover/accent:scale-105";
const SWATCH_SELECTED = "border-surface shadow-[var(--shadow-swatch-selected)]";

function SelectionMark({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden
      data-slot="accent-selection-mark"
      className={cn(
        "pointer-events-none absolute text-surface transition-[opacity,transform] duration-[var(--dur-fast)]",
        selected ? "scale-100 opacity-100" : "scale-75 opacity-0",
      )}
    >
      <Icon name="check" size="xs" />
    </span>
  );
}

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
    <label title={label} aria-label={label} className={SWATCH_TARGET}>
      <span
        aria-hidden
        className={cn(SWATCH_CHROME, isActive && SWATCH_SELECTED)}
        style={{ background: isActive ? value : RAINBOW_HINT }}
      />
      <SelectionMark selected={isActive} />
      {/* Hidden native input — the visible swatch is the label; click
          opens the OS color picker. */}
      <ColorPickerInput
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    </label>
  );
}

export function AccentSection() {
  const t = useT();
  const accents = useExtensionPoint(ACCENT);
  const { accent, setAccent, scheme } = useAccentPreference();
  // Swatch must paint the color that ACTUALLY applies in the current scheme:
  // presets carry a hand-tuned `light` variant applyTheme uses in light themes,
  // so painting the dark hex would show a color different from the one the app
  // renders. (The stored key stays the dark hex — the active check is unchanged.)
  const light = scheme === "light";

  const isCustom = !accents.some((a) => a.dark === accent);

  return (
    <SettingRow label={t("settings.accent")} sub={t("settings.accent.sub")}>
      <div className="flex flex-wrap gap-2.5 justify-start items-center">
        {accents.map((a) => (
          <Pressable
            key={a.id}
            type="button"
            onClick={() => setAccent(a.dark)}
            title={`${t("settings.accent")}: ${a.label}`}
            aria-label={`${t("settings.accent")}: ${a.label}`}
            aria-pressed={accent === a.dark}
            className={cn("p-0", SWATCH_TARGET)}
          >
            <span
              aria-hidden
              className={cn(SWATCH_CHROME, accent === a.dark && SWATCH_SELECTED)}
              style={{ background: light ? (a.light ?? a.dark) : a.dark }}
            />
            <SelectionMark selected={accent === a.dark} />
          </Pressable>
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
