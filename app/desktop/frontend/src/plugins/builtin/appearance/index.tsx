// Built-in plugin: Appearance settings pane.
//
// Lists every theme registered via `host.theme.registerTheme()` as a
// row with a live color preview (canvas / surface / accent), the
// theme's scheme (dark vs light), and a check on the active one.
// Adding a theme plugin makes a row appear here automatically — no
// changes to this file.

import { cn } from "@/lib/utils";
import { Icon } from "@/components/common";
import { LOCALES, setLocale, useLocale, useT, type Locale } from "@/lib/i18n";
import { definePlugin, useAccents, useThemes } from "@/plugins/sdk";
import type { ThemeSpec } from "@/plugins/sdk";
import { useThemeStore } from "@/state/themeStore";

// Fallback hexes for previewing themes that didn't ship a `tokens` map.
// Match the built-in dark palette so the preview never goes blank.
const FALLBACK_TOKENS: Record<string, Record<string, string>> = {
  dark:  { bg: "#010102", surface: "#181a1d", accent: "#1ed760" },
  light: { bg: "#fafafa", surface: "#ffffff", accent: "#15883e" },
};

function previewTokens(spec: ThemeSpec): { bg: string; surface: string; accent: string } {
  const fallback = FALLBACK_TOKENS[spec.scheme];
  return {
    bg:      spec.tokens?.["color-bg"]      ?? fallback.bg,
    surface: spec.tokens?.["color-surface"] ?? fallback.surface,
    accent:  spec.tokens?.["color-accent"]  ?? fallback.accent,
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
        <div className="truncate text-[13px] font-semibold leading-[1.2] text-fg">{spec.label}</div>
        <div className="inline-flex items-center gap-1 font-mono text-[11px] text-fg-faint lowercase tracking-normal">
          <Icon name={spec.scheme === "dark" ? "moon" : "sun"} size={10} className="shrink-0" />
          {spec.scheme}
        </div>
      </div>
      {active && <Icon name="check" size={14} className="shrink-0 text-accent" />}
    </button>
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

  return (
    <div>
      {/* Block-style row: label on top, control below at full width — the
          theme picker is a grid that needs more room than a 1fr column. */}
      <div className="grid items-stretch gap-3 py-3">
        <div>
          <div className="text-[13px] font-semibold text-fg">{t("settings.theme")}</div>
          <div className="mt-0.5 text-[11.5px] text-fg-faint">{t("settings.theme.sub")}</div>
        </div>
        <div className="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(220px,1fr))]">
          {themes.map((spec) => (
            <ThemeRow
              key={spec.id}
              spec={spec}
              active={theme === spec.id}
              onSelect={setTheme}
            />
          ))}
        </div>
      </div>

      {/* Default settings row: label column + control column. */}
      <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
        <div>
          <div className="text-[13px] font-semibold text-fg">{t("settings.accent")}</div>
          <div className="mt-0.5 text-[11.5px] text-fg-faint">{t("settings.accent.sub")}</div>
        </div>
        <div className="flex flex-wrap gap-2.5 justify-start">
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
        </div>
      </div>

      {/* Language picker — segmented control sized to fit the two
          built-in locales. Adding a new locale to lib/i18n.ts grows the
          control automatically. */}
      <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
        <div>
          <div className="text-[13px] font-semibold text-fg">{t("settings.language.label")}</div>
          <div className="mt-0.5 text-[11.5px] text-fg-faint">{t("settings.language.sub")}</div>
        </div>
        <div
          role="radiogroup"
          aria-label={t("settings.language.label")}
          className="inline-flex gap-1 rounded-md border border-line bg-surface-2 p-1"
        >
          {LOCALES.map((l) => (
            <button
              key={l.id}
              type="button"
              role="radio"
              aria-checked={locale === l.id}
              onClick={() => setLocale(l.id as Locale)}
              className={cn(
                "rounded-sm px-3 py-1 text-[12px] font-medium cursor-pointer transition-colors duration-150",
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
