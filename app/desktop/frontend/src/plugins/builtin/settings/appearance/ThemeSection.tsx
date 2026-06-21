// Theme picker. Rows come from the live theme registry — adding a
// theme plugin makes it show up here with no further wiring.

import type { ThemeSpec } from "@/plugins/sdk";
import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { THEME, useExtensionPoint } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

// Fallback hexes for previewing themes that didn't ship a `tokens` map.
// Match the built-in dark palette so the preview never goes blank.
// Typed as a concrete `Record<"dark" | "light", ...>` so indexed access
// returns the inner object as a typed struct (not Record<string,string>,
// which under noUncheckedIndexedAccess returns string | undefined).
const FALLBACK_TOKENS: Record<"dark" | "light", { bg: string; surface: string; accent: string }> = {
  dark: { bg: "#0c0d0f", surface: "#16181b", accent: "#6c97ff" },
  light: { bg: "#ffffff", surface: "#f6f7f8", accent: "#2563eb" },
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
        "grid grid-cols-[48px_minmax(0,1fr)_auto] items-center gap-3 rounded-md border border-line bg-surface px-3 py-2.5 text-left transition-[background,border-color] duration-150 hover:bg-surface-2",
        active && "bg-surface-2 border-accent",
      )}
    >
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

// "System" follows the OS appearance (the default). It isn't a registered
// THEME spec, so it gets its own row with a split dark/light preview.
function SystemRow({ active, onSelect }: { active: boolean; onSelect: () => void }) {
  const t = useT();
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={active}
      className={cn(
        "grid grid-cols-[48px_minmax(0,1fr)_auto] items-center gap-3 rounded-md border border-line bg-surface px-3 py-2.5 text-left transition-[background,border-color] duration-150 hover:bg-surface-2",
        active && "bg-surface-2 border-accent",
      )}
    >
      <div className="relative h-8 w-12 shrink-0 overflow-hidden rounded-sm border border-line">
        <div
          className="absolute inset-y-0 left-0 w-1/2"
          style={{ background: FALLBACK_TOKENS.dark.bg }}
        />
        <div
          className="absolute inset-y-0 right-0 w-1/2"
          style={{ background: FALLBACK_TOKENS.light.bg }}
        />
      </div>
      <div className="grid min-w-0 gap-0.5">
        <div className="truncate text-[14px] font-semibold leading-[1.2] text-fg">
          {t("settings.theme.system")}
        </div>
        <div className="inline-flex items-center gap-1 font-mono text-[11px] text-fg-faint lowercase tracking-normal">
          <Icon name="settings" size={10} className="shrink-0" />
          {t("settings.theme.systemSub")}
        </div>
      </div>
      {active && <Icon name="check" size={14} className="shrink-0 text-accent" />}
    </button>
  );
}

export function ThemeSection() {
  const t = useT();
  const themes = useExtensionPoint(THEME);
  const theme = useUiStore((s) => s.theme);
  const setTheme = useUiStore((s) => s.setTheme);

  return (
    // Full-width block (label on top, grid below) — the theme grid
    // needs more horizontal room than the standard 140px+1fr row.
    <div className="grid items-stretch gap-3 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{t("settings.theme")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.theme.sub")}</div>
      </div>
      <div className="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(220px,1fr))]">
        <SystemRow active={theme === "system"} onSelect={() => setTheme("system")} />
        {themes.map((spec) => (
          <ThemeRow key={spec.id} spec={spec} active={theme === spec.id} onSelect={setTheme} />
        ))}
      </div>
    </div>
  );
}
