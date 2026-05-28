// Font customization — UI + code typefaces and base size. Empty
// string reverts to the bundled Geist defaults; numeric `null` reverts
// size to the inherited 15px baseline.
//
// JetBrains IDEA / VS Code-style pattern: a checkbox toggles "use a
// custom font", and a Radix DropdownMenu picks from the curated list
// of fonts actually installed on the user's machine. Each item renders
// in its own family so the user sees a preview before clicking.

import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useSystemFonts } from "@/lib/systemFonts";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/state/uiStore";

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
  // Display name on the trigger: the chosen family, or the localized
  // "Default (Geist…)" placeholder when the checkbox is off.
  const triggerLabel = customEnabled ? value : defaultLabel;

  return (
    <div className="grid grid-cols-[60px_auto_1fr] items-center gap-2">
      <span className="text-[12px] font-semibold text-fg-faint">{label}</span>
      <label className="inline-flex cursor-pointer items-center gap-1.5 text-[12.5px] text-fg-muted">
        <input
          type="checkbox"
          aria-label={`Use custom ${label.toLowerCase()} font`}
          checked={customEnabled}
          onChange={(e) => onChange(e.target.checked ? (fonts[0] ?? "") : "")}
          className="h-3.5 w-3.5 cursor-pointer accent-accent"
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

const SIZE_OPTIONS = [13, 14, 15, 16, 17, 18] as const;

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
  return (
    <div className="grid grid-cols-[60px_1fr] items-center gap-2">
      <span className="text-[12px] font-semibold text-fg-faint">{label}</span>
      <div className="inline-flex w-fit items-center gap-1 rounded-md border border-line bg-surface-2 p-1">
        <button
          type="button"
          onClick={() => onChange(null)}
          className={cn(
            "rounded-sm px-2.5 py-0.5 text-[12px] cursor-pointer transition-colors",
            value === null
              ? "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
              : "bg-transparent text-fg-muted hover:text-fg",
          )}
        >
          {resetLabel}
        </button>
        {SIZE_OPTIONS.map((px) => (
          <button
            key={px}
            type="button"
            onClick={() => onChange(px)}
            className={cn(
              "rounded-sm px-2.5 py-0.5 font-mono text-[12px] tabular-nums cursor-pointer transition-colors",
              value === px
                ? "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                : "bg-transparent text-fg-muted hover:text-fg",
            )}
          >
            {px}
          </button>
        ))}
      </div>
    </div>
  );
}

export function FontSection() {
  const t = useT();
  const uiFont = useUiStore((s) => s.uiFont);
  const codeFont = useUiStore((s) => s.codeFont);
  const fontSize = useUiStore((s) => s.fontSize);
  const setUiFont = useUiStore((s) => s.setUiFont);
  const setCodeFont = useUiStore((s) => s.setCodeFont);
  const setFontSize = useUiStore((s) => s.setFontSize);

  return (
    <div className="grid grid-cols-[140px_1fr] items-start gap-4 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{t("settings.font")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.font.sub")}</div>
      </div>
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
      </div>
    </div>
  );
}
