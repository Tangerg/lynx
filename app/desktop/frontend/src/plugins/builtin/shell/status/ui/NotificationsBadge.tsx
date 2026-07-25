import { IconButton } from "@/ui";
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
    <IconButton
      icon="chat"
      size="sm"
      quiet
      badge={badgeText ?? undefined}
      title={
        unread > 0 ? t("status.notifications.unread", { count: unread }) : t("status.notifications")
      }
      aria-label={t("status.notifications")}
      onClick={onClick}
    />
  );
}
