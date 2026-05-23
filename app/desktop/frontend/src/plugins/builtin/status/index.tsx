// Built-in plugins: status-bar contributors.
//
// `statusPill` is the run-state + tokens + cost cluster (lives in its
// own file because it's a couple of components plus shorthand parsing).
// `statusNotifications` is a tiny unread-badge that opens the
// Notifications workspace view when clicked — kept inline here.

import { Icon } from "@/components/common";
import { definePlugin, useNotificationStore } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

export { statusPill } from "./pill";

function NotificationsBadge() {
  const unread = useNotificationStore((s) =>
    s.log.reduce((n, e) => (e.dismissed ? n : n + 1), 0),
  );

  const onClick = () => {
    useSessionStore.getState().openMainView({
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

export const statusNotifications = definePlugin({
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
