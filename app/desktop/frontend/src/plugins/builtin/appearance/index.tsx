// Built-in plugin: Appearance settings pane.
//
// Lists every theme registered via `host.theme.registerTheme()` as a
// row with a live color preview (canvas / surface / accent), the
// theme's scheme (dark vs light), and a check on the active one.
// Adding a theme plugin makes a row appear here automatically — no
// changes to this file.

import type {Locale} from "@/lib/i18n";
import type { ThemeSpec } from "@/plugins/sdk";
import { useState } from "react";
import { Icon } from "@/components/common";
import {  LOCALES, setLocale, useLocale, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin, useAccents, useThemes } from "@/plugins/sdk";
import { useThemeStore } from "@/state/themeStore";

// Fallback hexes for previewing themes that didn't ship a `tokens` map.
// Match the built-in dark palette so the preview never goes blank.
const FALLBACK_TOKENS: Record<string, Record<string, string>> = {
  dark: { bg: "#010102", surface: "#181a1d", accent: "#1ed760" },
  light: { bg: "#fafafa", surface: "#ffffff", accent: "#15883e" },
};

function previewTokens(spec: ThemeSpec): { bg: string; surface: string; accent: string } {
  const fallback = FALLBACK_TOKENS[spec.scheme];
  return {
    bg: spec.tokens?.["color-bg"] ?? fallback.bg,
    surface: spec.tokens?.["color-surface"] ?? fallback.surface,
    accent: spec.tokens?.["color-accent"] ?? fallback.accent,
  };
}

function ThemeRow({
  spec,
  active,
  onSelect,
}: {
  spec: ThemeSpec;
  active: boolean;
  onSelect: (id: string) => void;
}) {
  const preview = previewTokens(spec);
  return (
    <button
      type="button"
      onClick={() => onSelect(spec.id)}
      aria-pressed={active}
      className={cn(
        "grid grid-cols-[48px_minmax(0,1fr)_auto] items-center gap-3 rounded-md border border-line bg-surface px-3 py-2.5 text-left cursor-pointer transition-[background,border-color] duration-150 hover:bg-surface-2 focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_22%,transparent)]",
        active && "bg-surface-2 border-accent",
      )}
    >
      {/* Layered swatch: bg fills the rectangle, surface lifts above as a
          floating tile, accent dot anchors the bottom-right. Reads as a
          mini "what does this theme look like" cue without needing words. */}
      <div
        className="relative h-8 w-12 shrink-0 overflow-hidden rounded-sm border border-line"
        style={{ background: preview.bg }}
      >
        <div
          className="absolute inset-x-2 top-2 bottom-1 rounded-[2px]"
          style={{ background: preview.surface }}
        />
        <div
          className="absolute right-1 bottom-1 h-1.5 w-1.5 rounded-full"
          style={{ background: preview.accent }}
        />
      </div>
      <div className="grid min-w-0 gap-0.5">
        <div className="truncate text-[14px] font-semibold leading-[1.2] text-fg">{spec.label}</div>
        <div className="inline-flex items-center gap-1 font-mono text-[11px] text-fg-faint lowercase tracking-normal">
          <Icon name={spec.scheme === "dark" ? "moon" : "sun"} size={10} className="shrink-0" />
          {spec.scheme}
        </div>
      </div>
      {active && <Icon name="check" size={14} className="shrink-0 text-accent" />}
    </button>
  );
}

// Conic gradient used when no custom color is active — communicates
// "click me, you can pick anything" without committing to a default hue.
const RAINBOW_HINT = "conic-gradient(from 0deg, #ef4444, #f59e0b, #eab308, #22c55e, #06b6d4, #6366f1, #a855f7, #ec4899, #ef4444)";

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
        "relative inline-grid h-4.5 w-4.5 place-items-center rounded-full border-2 border-transparent bg-clip-padding cursor-pointer transition-[transform,box-shadow] duration-150 hover:scale-[1.08] active:scale-95",
        isActive && "border-surface shadow-[0_0_0_1.5px_var(--color-text)]",
      )}
      style={{ background: isActive ? value : RAINBOW_HINT }}
    >
      {/* Native color input — visually hidden but consumes the click and
          opens the OS color picker (macOS color wheel, etc.). */}
      <input
        type="color"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="absolute inset-0 h-full w-full cursor-pointer opacity-0"
      />
    </label>
  );
}

// Free-text font picker. Commits to the store on blur / Enter so the
// user can type a multi-word name without each keystroke re-rendering
// the whole app. Empty string = reset to bundled default.
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

function AppearancePane() {
  const t = useT();
  const locale = useLocale();
  const theme = useThemeStore((s) => s.theme);
  const accent = useThemeStore((s) => s.accent);
  const setTheme = useThemeStore((s) => s.setTheme);
  const setAccent = useThemeStore((s) => s.setAccent);

  const themes = useThemes();
  const accents = useAccents();
  const uiFont = useThemeStore((s) => s.uiFont);
  const codeFont = useThemeStore((s) => s.codeFont);
  const fontSize = useThemeStore((s) => s.fontSize);
  const messageStyle = useThemeStore((s) => s.messageStyle);
  const setUiFont = useThemeStore((s) => s.setUiFont);
  const setCodeFont = useThemeStore((s) => s.setCodeFont);
  const setFontSize = useThemeStore((s) => s.setFontSize);
  const setMessageStyle = useThemeStore((s) => s.setMessageStyle);

  return (
    <div>
      {/* Block-style row: label on top, control below at full width — the
          theme picker is a grid that needs more room than a 1fr column. */}
      <div className="grid items-stretch gap-3 py-3">
        <div>
          <div className="text-[15px] font-semibold text-fg">{t("settings.theme")}</div>
          <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.theme.sub")}</div>
        </div>
        <div className="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(220px,1fr))]">
          {themes.map((spec) => (
            <ThemeRow key={spec.id} spec={spec} active={theme === spec.id} onSelect={setTheme} />
          ))}
        </div>
      </div>

      {/* Default settings row: label column + control column. */}
      <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
        <div>
          <div className="text-[15px] font-semibold text-fg">{t("settings.accent")}</div>
          <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.accent.sub")}</div>
        </div>
        <div className="flex flex-wrap gap-2.5 justify-start items-center">
          {accents.map((a) => (
            <button
              key={a.id}
              type="button"
              onClick={() => setAccent(a.dark)}
              title={`${t("settings.accent")}: ${a.label}`}
              aria-label={`${t("settings.accent")}: ${a.label}`}
              aria-pressed={accent === a.dark}
              style={{ background: a.dark }}
              className={cn(
                "h-4.5 w-4.5 rounded-full border-2 border-transparent bg-clip-padding p-0 cursor-pointer transition-[transform,box-shadow] duration-150 hover:scale-[1.08] active:scale-95",
                accent === a.dark && "border-surface shadow-[0_0_0_1.5px_var(--color-text)]",
              )}
            />
          ))}
          {/* Custom color picker — opens the OS-native color wheel via a
              hidden <input type="color">. The swatch shows the current
              accent when it's not one of the registered presets, and a
              rainbow gradient hint otherwise. */}
          <CustomAccentPicker
            value={accent}
            isActive={!accents.some((a) => a.dark === accent)}
            onChange={setAccent}
            label={t("settings.accent.custom")}
          />
        </div>
      </div>

      {/* Font customization — UI / Code typefaces + base size. Empty
          input string reverts to the bundled Geist defaults. */}
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

      {/* Message style — bubble (right-aligned card) or plain
          (left-aligned, no card). */}
      <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
        <div>
          <div className="text-[15px] font-semibold text-fg">
            {t("settings.messageStyle")}
          </div>
          <div className="mt-0.5 text-[13px] text-fg-muted">
            {t("settings.messageStyle.sub")}
          </div>
        </div>
        <div
          role="radiogroup"
          aria-label={t("settings.messageStyle")}
          className="inline-flex w-fit gap-1 rounded-md border border-line bg-surface-2 p-1"
        >
          {(["bubble", "plain"] as const).map((s) => (
            <button
              key={s}
              type="button"
              role="radio"
              aria-checked={messageStyle === s}
              onClick={() => setMessageStyle(s)}
              className={cn(
                "rounded-sm px-3 py-1 text-[13px] font-medium cursor-pointer transition-colors duration-150",
                messageStyle === s
                  ? "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                  : "bg-transparent text-fg-muted hover:text-fg",
              )}
            >
              {t(`settings.messageStyle.${s}`)}
            </button>
          ))}
        </div>
      </div>

      {/* Language picker — segmented control sized to fit the two
          built-in locales. Adding a new locale to lib/i18n.ts grows the
          control automatically. */}
      <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
        <div>
          <div className="text-[15px] font-semibold text-fg">{t("settings.language.label")}</div>
          <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.language.sub")}</div>
        </div>
        <div
          role="radiogroup"
          aria-label={t("settings.language.label")}
          // `w-fit` keeps the radiogroup at content width inside the
          // 1fr grid cell. Without it, grid items default to
          // `justify-self: stretch`, which overrides `inline-flex`'s
          // shrink-to-content behaviour and the segmented control
          // gets dragged across the entire row.
          className="inline-flex w-fit gap-1 rounded-md border border-line bg-surface-2 p-1"
        >
          {LOCALES.map((l) => (
            <button
              key={l.id}
              type="button"
              role="radio"
              aria-checked={locale === l.id}
              onClick={() => setLocale(l.id as Locale)}
              className={cn(
                "rounded-sm px-3 py-1 text-[13px] font-medium cursor-pointer transition-colors duration-150",
                locale === l.id
                  ? "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                  : "bg-transparent text-fg-muted hover:text-fg",
              )}
            >
              {l.label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.appearance",
  version: "1.0.0",
  setup({ host }) {
    host.settings.registerPane({
      id: "appearance",
      label: "Appearance",
      icon: "spark",
      order: 0,
      component: AppearancePane,
    });
  },
});
