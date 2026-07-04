export interface StatusNotification {
  dismissed?: boolean;
}

export function unreadNotificationCount(notifications: readonly StatusNotification[]): number {
  return notifications.reduce((count, notification) => count + (notification.dismissed ? 0 : 1), 0);
}

export function notificationBadgeText(unreadCount: number): string | null {
  if (unreadCount <= 0) return null;
  return unreadCount > 9 ? "9+" : String(unreadCount);
}
