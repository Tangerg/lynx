// Font customization — UI + code typefaces and base size. Empty
// input string reverts to the bundled Geist defaults; numeric `null`
// reverts size to the inherited 15px baseline.

import { useState } from "react";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useThemeStore } from "@/state/themeStore";

// Free-text font picker. Commits on blur / Enter so the user can type
// a multi-word name without each keystroke re-rendering the whole app.
function FontField({
  label,
  placeholder,
  value,
  onCommit,
  mono,
}: {
  label: string;
  placeholder: string;
  value: string;
  onCommit: (v: string) => void;
  mono?: boolean;
}) {
  const [draft, setDraft] = useState(value);
  return (
    <label className="grid grid-cols-[60px_1fr] items-center gap-2">
      <span className="text-[12px] font-semibold text-fg-faint">{label}</span>
      <input
        type="text"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => onCommit(draft.trim())}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onCommit(draft.trim());
            (e.target as HTMLInputElement).blur();
          }
        }}
        placeholder={placeholder}
        spellCheck={false}
        style={{ fontFamily: draft ? `"${draft}"` : undefined }}
        className={cn(
          "h-8 w-full max-w-[280px] rounded-md border border-line bg-surface px-2.5 text-[13px] text-fg outline-none placeholder:text-fg-faint",
          "focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]",
          mono && "font-mono text-[12.5px]",
        )}
      />
    </label>
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
  const uiFont = useThemeStore((s) => s.uiFont);
  const codeFont = useThemeStore((s) => s.codeFont);
  const fontSize = useThemeStore((s) => s.fontSize);
  const setUiFont = useThemeStore((s) => s.setUiFont);
  const setCodeFont = useThemeStore((s) => s.setCodeFont);
  const setFontSize = useThemeStore((s) => s.setFontSize);

  return (
    <div className="grid grid-cols-[140px_1fr] items-start gap-4 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{t("settings.font")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.font.sub")}</div>
      </div>
      <div className="grid gap-2">
        <FontField
          label={t("settings.font.ui")}
          placeholder="Geist"
          value={uiFont}
          onCommit={setUiFont}
        />
        <FontField
          label={t("settings.font.code")}
          placeholder="Geist Mono"
          value={codeFont}
          onCommit={setCodeFont}
          mono
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
