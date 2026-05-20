// Built-in plugin: contributes the user-card footer (avatar + name +
// email + settings cog + account menu) into the sidebar.footer slot.
//
// The settings cog used to open a small popover with theme/accent
// quick-toggles + a "More settings…" link. That popover is gone now:
// clicking the cog goes straight to the Settings page (a workspace
// view in the main tab strip). Theme and accent live inside the
// Appearance pane there.

import { Icon } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function SidebarFooter() {
  const openSettings = () =>
    useUIStore.getState().openMainView({
      id: "settings",
      title: "Settings",
      icon: "settings",
    });

  return (
    <div className="user-card">
      <div className="user-avatar">J</div>
      <div className="user-body">
        <div className="user-name">Jamie Doe</div>
        <div className="user-sub">jdoe@longbridge-inc.com</div>
      </div>
      <button className="user-action" onClick={openSettings} title="Settings">
        <Icon name="settings" size={14} />
      </button>
      <button className="user-action" title="Account menu">
        <Icon name="more" size={14} />
      </button>
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
