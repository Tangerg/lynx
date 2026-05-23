// Built-in plugin: Appearance settings pane.
//
// Lists every theme registered via `host.theme.registerTheme()` as a
// row with a live color preview (canvas / surface / accent), the
// theme's scheme (dark vs light), and a check on the active one.
// Adding a theme plugin makes a row appear here automatically — no
// changes to this file.

import { cn } from "@/lib/utils";
import { Icon } from "@/components/common";
import { definePlugin, useAccents, useThemes } from "@/plugins/sdk";
import type { ThemeSpec } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

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
      className={cn("theme-picker-row", active && "active")}
      onClick={() => onSelect(spec.id)}
      aria-pressed={active}
    >
      {/* Layered swatch: bg fills the rectangle, surface lifts above as a
          floating tile, accent dot anchors the bottom-right. Reads as a
          mini "what does this theme look like" cue without needing words. */}
      <div className="theme-picker-swatch" style={{ background: preview.bg }}>
        <div className="tps-surface" style={{ background: preview.surface }} />
        <div className="tps-accent" style={{ background: preview.accent }} />
      </div>
      <div className="theme-picker-meta">
        <div className="theme-picker-name">{spec.label}</div>
        <div className="theme-picker-scheme">
          <Icon name={spec.scheme === "dark" ? "moon" : "sun"} size={10} />
          {spec.scheme}
        </div>
      </div>
      {active && <Icon name="check" size={14} className="theme-picker-check" />}
    </button>
  );
}

function AppearancePane() {
  const theme = useUIStore((s) => s.theme);
  const accent = useUIStore((s) => s.accent);
  const setTheme = useUIStore((s) => s.setTheme);
  const setAccent = useUIStore((s) => s.setAccent);

  const themes = useThemes();
  const accents = useAccents();

  return (
    <div>
      <div className="settings-row settings-row-block">
        <div>
          <div className="settings-row-label">Theme</div>
          <div className="settings-row-sub">
            Pick a color theme. Plugins can register more — they show up here automatically.
          </div>
        </div>
        <div className="theme-picker">
          {themes.map((t) => (
            <ThemeRow
              key={t.id}
              spec={t}
              active={theme === t.id}
              onSelect={setTheme}
            />
          ))}
        </div>
      </div>

      <div className="settings-row">
        <div>
          <div className="settings-row-label">Accent</div>
          <div className="settings-row-sub">Functional highlight color — play / active / CTA.</div>
        </div>
        <div className="accent-swatches" style={{ justifyContent: "flex-start" }}>
          {accents.map((a) => (
            <button
              key={a.id}
              className={`accent-swatch ${accent === a.dark ? "active" : ""}`}
              style={{ background: a.dark }}
              onClick={() => setAccent(a.dark)}
              title={`Accent: ${a.label}`}
            />
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
