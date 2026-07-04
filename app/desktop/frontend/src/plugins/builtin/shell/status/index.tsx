// Built-in plugins: status contributors.
//
// `statusNotifications` is the unread-count badge that lives in the
// sidebar footer (avatar row) and opens the Notifications view on click.

import { Icon, Tooltip } from "@/ui";
import { useT } from "@/lib/i18n";
import { openWorkspaceView } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin, useNotificationStore } from "@/plugins/sdk";

export { completionNotify } from "./completionNotify";
export { windowTitle } from "./windowTitle";

function NotificationsBadge() {
  const t = useT();
  const unread = useNotificationStore((s) => s.log.reduce((n, e) => (e.dismissed ? n : n + 1), 0));

  const onClick = () => {
    openWorkspaceView({
      id: "notifications",
      title: "workspace.view.title.notifications",
      icon: "chat",
    });
  };

  return (
    <Tooltip
      label={
        unread > 0 ? t("status.notifications.unread", { count: unread }) : t("status.notifications")
      }
    >
      <button
        type="button"
        onClick={onClick}
        aria-label={t("status.notifications")}
        className="relative grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint transition-[background,color] hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3 active:scale-[0.96]"
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
