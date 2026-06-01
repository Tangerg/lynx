// Built-in plugins: status contributors.
//
// `statusPill` is the run-telemetry cluster for the composer footer (its
// own file — a couple of components plus shorthand parsing).
// `statusNotifications` is the unread-count badge that lives in the
// sidebar footer (avatar row) and opens the Notifications view on click.

import { Icon, Tooltip } from "@/components/common";
import { definePlugin, useNotificationStore } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

export { statusPill } from "./pill";

function NotificationsBadge() {
  const unread = useNotificationStore((s) => s.log.reduce((n, e) => (e.dismissed ? n : n + 1), 0));

  const onClick = () => {
    useSessionStore.getState().openMainView({
      id: "notifications",
      title: "Notifications",
      icon: "chat",
    });
  };

  return (
    <Tooltip label={unread > 0 ? `${unread} unread notification(s)` : "Notifications"}>
      <button
        type="button"
        onClick={onClick}
        aria-label="Notifications"
        className="relative grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint cursor-pointer transition-[background,color] hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3 active:scale-[0.92]"
      >
        <Icon name="chat" size={14} />
        {unread > 0 && (
          <span className="absolute -right-0.5 -top-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-accent px-0.5 font-mono text-[9px] font-semibold text-on-accent">
            {unread > 9 ? "9+" : unread}
          </span>
        )}
      </button>
    </Tooltip>
  );
}

export const statusNotifications = definePlugin({
  name: "lyra.builtin.status-notifications",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer.status", {
      id: "notifications",
      order: 10,
      component: NotificationsBadge,
    });
  },
});
