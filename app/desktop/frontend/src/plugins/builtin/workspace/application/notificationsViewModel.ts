import type { Translate } from "@/lib/i18n";
import type { NotificationEntry, NotificationLevel } from "@/plugins/sdk";

export type NotificationDotTone = "err" | "waiting" | "idle";

export interface NotificationsViewModel {
  entries: NotificationEntry[];
  unreadCount: number;
  totalCount: number;
  isEmpty: boolean;
}

const DOT_TONE_BY_LEVEL: Record<NotificationLevel, NotificationDotTone> = {
  error: "err",
  warn: "waiting",
  info: "idle",
};

export function notificationsViewModel(log: readonly NotificationEntry[]): NotificationsViewModel {
  const entries = Array.from(log).reverse();
  let unreadCount = 0;
  for (const entry of entries) {
    if (!entry.dismissed) {
      unreadCount += 1;
    }
  }

  return {
    entries,
    unreadCount,
    totalCount: entries.length,
    isEmpty: entries.length === 0,
  };
}

export function notificationsSubtext(
  t: Translate,
  { unreadCount, totalCount }: Pick<NotificationsViewModel, "unreadCount" | "totalCount">,
): string {
  return t("notifications.subtext", { unread: unreadCount, total: totalCount });
}

export function notificationDotTone(level: NotificationLevel): NotificationDotTone {
  return DOT_TONE_BY_LEVEL[level];
}
