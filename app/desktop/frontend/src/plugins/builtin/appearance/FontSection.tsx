// Font customization — UI + code typefaces and base size. Empty
// string reverts to the bundled Geist defaults; numeric `null` reverts
// size to the inherited 15px baseline.
//
// JetBrains IDEA / VS Code-style pattern: a checkbox toggles "use a
// custom font", and a Radix DropdownMenu picks from the curated list
// of fonts actually installed on the user's machine. Each item renders
// in its own family so the user sees a preview before clicking.

import type { SegmentedOption } from "@/components/common";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useId } from "react";
import { Checkbox, Icon, Segmented } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useSystemFonts } from "@/lib/systemFonts";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/state/uiStore";
import { SettingRow } from "./SettingRow";

interface FontPickerProps {
  label: string;
  mono: boolean;
  value: string;
  onChange: (v: string) => void;
  defaultLabel: string;
}

function FontPicker({ label, mono, value, onChange, defaultLabel }: FontPickerProps) {
  const fonts = useSystemFonts(mono);
  const customEnabled = value !== "";
  const checkboxId = useId();
  // Display name on the trigger: the chosen family, or the localized
  // "Default (Geist…)" placeholder when the checkbox is off.
  const triggerLabel = customEnabled ? value : defaultLabel;

  return (
    <div className="grid grid-cols-[60px_auto_1fr] items-center gap-2">
      <span className="text-[12px] font-semibold text-fg-faint">{label}</span>
      <label
        htmlFor={checkboxId}
        className="inline-flex cursor-pointer items-center gap-1.5 text-[12.5px] text-fg-muted"
      >
        <Checkbox
          id={checkboxId}
          ariaLabel={`Use custom ${label.toLowerCase()} font`}
          checked={customEnabled}
          onCheckedChange={(c) => onChange(c ? (fonts[0] ?? "") : "")}
        />
        <span>Use custom</span>
      </label>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          disabled={!customEnabled}
          className={cn(
            "inline-flex w-fit min-w-[220px] max-w-[280px] items-center justify-between gap-2 rounded-md border border-line bg-surface px-2.5 py-1.5 text-[13px] text-fg cursor-pointer transition-colors hover:bg-surface-2 data-[state=open]:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
            "disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-surface",
            mono && customEnabled && "font-mono text-[12.5px]",
          )}
          style={customEnabled ? { fontFamily: `"${value}"` } : undefined}
        >
          <span className="truncate">{triggerLabel}</span>
          <Icon name="more" size={11} className="shrink-0 text-fg-faint -rotate-90" />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            align="start"
            sideOffset={4}
            className="z-50 max-h-[280px] min-w-[220px] overflow-auto rounded-md border border-line-soft bg-surface p-1 shadow-lg animate-rise-in"
          >
            {fonts.map((f) => (
              <DropdownMenu.Item
                key={f}
                onSelect={() => onChange(f)}
                // Preview each option in its own family — the user can
                // scan the list and pick by visual feel, not by name
                // recall.
                style={{ fontFamily: `"${f}"` }}
                className="grid cursor-pointer grid-cols-[minmax(0,1fr)_12px] items-center gap-2 rounded-sm px-2.5 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
              >
                <span className="truncate">{f}</span>
                {value === f ? (
                  <Icon name="check" size={12} className="text-accent" />
                ) : (
                  <span aria-hidden />
                )}
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  );
}

const SIZE_VALUES = [13, 14, 15, 16, 17, 18] as const;
// "default" sentinel = revert to the inherited 15px baseline (null in store).
const SIZE_RESET = "default";

function FontSizeField({
  label,
  value,
  onChange,
  resetLabel,
}: {
  label: string;
  value: number | null;
  onChange: (v: number | null) => void;
  resetLabel: string;
}) {
  const options: SegmentedOption<string>[] = [
    { value: SIZE_RESET, label: resetLabel },
    ...SIZE_VALUES.map((px) => ({ value: String(px), label: String(px) })),
  ];
  return (
    <div className="grid grid-cols-[60px_1fr] items-center gap-2">
      <span className="text-[12px] font-semibold text-fg-faint">{label}</span>
      <Segmented
        value={value === null ? SIZE_RESET : String(value)}
        options={options}
        onChange={(v) => onChange(v === SIZE_RESET ? null : Number(v))}
        ariaLabel={label}
        mono
      />
    </div>
  );
}

export function FontSection() {
  const t = useT();
  const uiFont = useUiStore((s) => s.uiFont);
  const codeFont = useUiStore((s) => s.codeFont);
  const fontSize = useUiStore((s) => s.fontSize);
  const fontSmoothing = useUiStore((s) => s.fontSmoothing);
  const setUiFont = useUiStore((s) => s.setUiFont);
  const setCodeFont = useUiStore((s) => s.setCodeFont);
  const setFontSize = useUiStore((s) => s.setFontSize);
  const setFontSmoothing = useUiStore((s) => s.setFontSmoothing);
  const smoothingId = useId();

  return (
    <SettingRow label={t("settings.font")} sub={t("settings.font.sub")} align="start">
      <div className="grid gap-2">
        <FontPicker
          label={t("settings.font.ui")}
          mono={false}
          value={uiFont}
          onChange={setUiFont}
          defaultLabel="Default (Geist)"
        />
        <FontPicker
          label={t("settings.font.code")}
          mono={true}
          value={codeFont}
          onChange={setCodeFont}
          defaultLabel="Default (Geist Mono)"
        />
        <FontSizeField
          label={t("settings.font.size")}
          value={fontSize}
          onChange={setFontSize}
          resetLabel={t("settings.font.default")}
        />
        <label
          htmlFor={smoothingId}
          className="mt-1 inline-flex cursor-pointer items-center gap-2 text-[12.5px] text-fg-muted"
        >
          <Checkbox
            id={smoothingId}
            ariaLabel={t("settings.font.smoothing")}
            checked={fontSmoothing}
            onCheckedChange={setFontSmoothing}
          />
          <span>{t("settings.font.smoothing")}</span>
        </label>
      </div>
    </SettingRow>
  );
}
