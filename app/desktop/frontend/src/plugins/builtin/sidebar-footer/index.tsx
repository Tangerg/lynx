// Built-in plugin: contributes the user-card footer (avatar + name + email +
// settings cog + account menu) into the sidebar.footer slot.
//
// The settings cog opens the SettingsPopover (theme + accent + "More
// settings…" jump). A future sign-in plugin can replace this entire block
// with its own UI by registering another contribution on the same slot —
// last-wins replaces this one.

import { useState } from "react";
import { Icon } from "@/components/common";
import { SettingsPopover } from "@/components/sidebar/SettingsPopover";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function SidebarFooter() {
  const [settingsOpen, setSettingsOpen] = useState(false);

  // Theme + accent read here so the SettingsPopover doesn't have to drill
  // them through any intermediate component.
  const theme = useUIStore((s) => s.theme);
  const accent = useUIStore((s) => s.accent);
  const toggleTheme = useUIStore((s) => s.toggleTheme);
  const setAccent = useUIStore((s) => s.setAccent);

  return (
    <div className="user-card">
      <div className="user-avatar">J</div>
      <div className="user-body">
        <div className="user-name">Jamie Doe</div>
        <div className="user-sub">jdoe@longbridge-inc.com</div>
      </div>
      <div className="user-settings-wrap">
        <button
          className={`user-action ${settingsOpen ? "open" : ""}`}
          onClick={() => setSettingsOpen((o) => !o)}
          title="Preferences"
        >
          <Icon name="settings" size={14} />
        </button>
        {settingsOpen && (
          <SettingsPopover
            theme={theme}
            accent={accent}
            onToggleTheme={toggleTheme}
            onAccentChange={setAccent}
            onClose={() => setSettingsOpen(false)}
          />
        )}
      </div>
      <button className="user-action" title="Account menu"><Icon name="more" size={14} /></button>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.sidebar-footer",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer", {
      id: "user-card",
      order: 0,
      component: SidebarFooter,
    });
  },
});
