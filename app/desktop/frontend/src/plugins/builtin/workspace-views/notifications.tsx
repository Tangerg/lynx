// Built-in plugin: "Notifications" workspace view — the persistent feed
// behind every `host.notify(...)` call.

import { EmptyState, Icon, IconButton, ScrollArea } from "@/components/common";
import { ViewHeader } from "@/components/views/ViewHeader";
import { definePlugin, useNotificationStore } from "@/plugins/sdk";

function timeAgo(ts: number): string {
  const sec = Math.floor((Date.now() - ts) / 1000);
  if (sec < 5) return "just now";
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  return `${day}d ago`;
}

function NotificationsTab() {
  const log = useNotificationStore((s) => s.log);
  const dismiss = useNotificationStore((s) => s.dismiss);
  const clearAll = useNotificationStore((s) => s.clearAll);

  // Newest first.
  const entries = [...log].reverse();
  const visible = entries.filter((e) => !e.dismissed);

  return (
    <>
      <ViewHeader
        icon="chat"
        titleStrong
        title="Notifications"
        sub={`${visible.length} unread · ${entries.length} total`}
        actions={
          <IconButton title="Clear all" onClick={clearAll}>
            <Icon name="x" size={14} />
          </IconButton>
        }
      />
      <ScrollArea style={{ padding: "4px 0" }}>
        {entries.length === 0 && (
          <EmptyState
            icon="chat"
            title="No notifications"
            sub="Anything a plugin reports via host.notify() will appear here."
          />
        )}
        {entries.map((e) => (
          <NotificationRow
            key={e.id}
            level={e.level}
            message={e.message}
            plugin={e.plugin}
            timestamp={e.timestamp}
            dismissed={e.dismissed}
            onDismiss={() => dismiss(e.id)}
          />
        ))}
      </ScrollArea>
    </>
  );
}

type RowProps = {
  level: "info" | "warn" | "error";
  message: string;
  plugin: string;
  timestamp: number;
  dismissed?: boolean;
  onDismiss: () => void;
};

function NotificationRow({
  level, message, plugin, timestamp, dismissed, onDismiss,
}: RowProps) {
  const dotColor =
    level === "error" ? "var(--color-error, #f87171)" :
    level === "warn"  ? "var(--color-warn,  #fbbf24)" :
                        "var(--color-text-faint)";

  return (
    <div
      className={`notification-row ${dismissed ? "dismissed" : ""} ${level}`}
      style={{
        padding: "8px 14px",
        display: "flex",
        gap: 10,
        alignItems: "flex-start",
        opacity: dismissed ? 0.5 : 1,
        borderBottom: "1px solid var(--color-border-soft)",
      }}
    >
      <div style={{
        width: 6, height: 6, borderRadius: "50%",
        marginTop: 6, background: dotColor, flexShrink: 0,
      }} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{
          fontSize: 12, color: "var(--color-text)",
          whiteSpace: "pre-wrap", wordBreak: "break-word",
        }}>
          {message}
        </div>
        <div style={{
          fontSize: 10, color: "var(--color-text-faint)", marginTop: 3,
        }}>
          {plugin} · {timeAgo(timestamp)}
        </div>
      </div>
      {!dismissed && (
        <IconButton title="Dismiss" onClick={onDismiss}>
          <Icon name="x" size={12} />
        </IconButton>
      )}
    </div>
  );
}

export const notificationsView = definePlugin({
  name: "lyra.builtin.view-notifications",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "notifications",
      title: "Notifications",
      icon: "chat",
      openByDefault: false,
      order: 50,
      component: NotificationsTab,
    });
  },
});
