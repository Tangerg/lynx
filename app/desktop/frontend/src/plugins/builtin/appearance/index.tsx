// Built-in plugin: Appearance settings pane.
//
// Same surface as the small popover above the user-card, but rendered inside
// the larger Settings modal — more breathing room + easier to expand later
// (font size, density, motion-reduction, etc.).
//
// All theme + accent options come from the plugin registry — see
// `lyra.builtin.default-themes` for the actual values. Power users can
// override or extend with their own theme plugin.

import { Icon, Segmented, type IconName } from "@/components/common";
import type { Theme } from "@/components/sidebar/types";
import { definePlugin, useAccents, useThemes } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function AppearancePane() {
  const theme = useUIStore((s) => s.theme);
  const accent = useUIStore((s) => s.accent);
  const setTheme = useUIStore((s) => s.setTheme);
  const setAccent = useUIStore((s) => s.setAccent);

  const themes = useThemes();
  const accents = useAccents();

  return (
    <div>
      <div className="settings-row">
        <div>
          <div className="settings-row-label">Theme</div>
          <div className="settings-row-sub">Switch the entire UI between dark and light.</div>
        </div>
        <div>
          <Segmented<Theme>
            value={theme}
            onChange={setTheme}
            options={themes.map((t) => ({
              value: t.id as Theme,
              label: (
                <>
                  {t.icon && <Icon name={t.icon as IconName} size={11} />}
                  {t.label}
                </>
              ),
            }))}
          />
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
