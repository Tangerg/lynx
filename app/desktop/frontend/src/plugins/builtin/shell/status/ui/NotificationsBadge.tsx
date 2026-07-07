import { Icon, Tooltip } from "@/ui";
import { useT } from "@/lib/i18n";
import { openWorkspaceView } from "@/plugins/builtin/workspace/public/navigation";
import { useNotificationStore } from "@/plugins/sdk";
import {
  notificationBadgeText,
  unreadNotificationCount,
} from "../application/notificationsReadout";

export function NotificationsBadge() {
  const t = useT();
  const unread = useNotificationStore((state) => unreadNotificationCount(state.log));
  const badgeText = notificationBadgeText(unread);

  const onClick = () => {
    openWorkspaceView("notifications");
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
        {badgeText && (
          <span className="absolute -right-0.5 -top-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-accent px-0.5 font-mono text-[9px] font-semibold text-on-accent">
            {badgeText}
          </span>
        )}
      </button>
    </Tooltip>
  );
}
