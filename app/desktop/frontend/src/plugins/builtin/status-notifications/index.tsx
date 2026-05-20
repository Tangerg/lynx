// Built-in plugin: unread-notification badge on the right side of the
// status bar. Clicking it opens the Notifications view in the main area.

import { Icon } from "@/components/common";
import { definePlugin, useNotificationStore } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function NotificationsBadge() {
  const unread = useNotificationStore((s) =>
    s.log.reduce((n, e) => (e.dismissed ? n : n + 1), 0),
  );

  const onClick = () => {
    useUIStore.getState().openMainView({
      id: "notifications",
      title: "Notifications",
      icon: "chat",
    });
  };

  return (
    <button
      className={`sb-item sb-btn ${unread > 0 ? "warn" : ""}`}
      onClick={onClick}
      title={unread > 0 ? `${unread} unread notification(s)` : "Notifications"}
    >
      <Icon name="chat" size={11} />
      {unread > 0 && <span>{unread}</span>}
    </button>
  );
}

export default definePlugin({
  name: "lyra.builtin.status-notifications",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.statusbar", {
      id: "notifications",
      order: 220,
      component: NotificationsBadge,
    });
  },
});
