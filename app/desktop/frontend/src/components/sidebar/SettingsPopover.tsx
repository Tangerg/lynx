import { motion } from "motion/react";
import { Icon, Segmented, type IconName } from "@/components/common";
import { popIn } from "@/lib/motion";
import { useAccents, useThemes } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";
import type { Theme } from "./types";

type Props = {
  theme: Theme;
  accent: string;
  onToggleTheme: () => void;
  onAccentChange: (color: string) => void;
  onClose: () => void;
};

// The small popover above the user-card. Both options (theme + accent)
// come from the plugin registry — no hardcoded list. `theme` is still a
// "dark"|"light" string for backward compat with the persisted store.
export function SettingsPopover({
  theme, accent, onToggleTheme, onAccentChange, onClose,
}: Props) {
  const openSettings = useUIStore((s) => s.openSettings);
  const themes = useThemes();
  const accents = useAccents();

  return (
    <motion.div className="settings-popover" onMouseLeave={onClose} {...popIn}>
      <div className="sp-section">Appearance</div>
      <div className="sp-row">
        <span className="sp-label">Theme</span>
        <Segmented<Theme>
          value={theme}
          onChange={(next) => { if (next !== theme) onToggleTheme(); }}
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
      <div className="sp-row">
        <span className="sp-label">Accent</span>
        <div className="accent-swatches">
          {accents.map((a) => (
            <button
              key={a.id}
              className={`accent-swatch ${accent === a.dark ? "active" : ""}`}
              style={{ background: a.dark }}
              onClick={() => onAccentChange(a.dark)}
              title={`Accent: ${a.label}`}
            />
          ))}
        </div>
      </div>
      <button
        className="sp-more"
        onClick={() => {
          onClose();
          openSettings();
        }}
      >
        <Icon name="settings" size={12} />
        <span>More settings…</span>
      </button>
    </motion.div>
  );
}
