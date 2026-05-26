// Built-in plugin: "Notifications" workspace view — the persistent feed
// behind every `host.notify(...)` call.

import { EmptyState, Icon, IconButton, ScrollArea } from "@/components/common";
import { ViewHeader } from "@/components/views/ViewHeader";
import { cn } from "@/lib/utils";
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
      <ScrollArea className="py-1">
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

interface RowProps {
  level: "info" | "warn" | "error";
  message: string;
  plugin: string;
  timestamp: number;
  dismissed?: boolean;
  onDismiss: () => void;
}

// Level → dot color. Lookup table beats a nested ternary and makes
// adding a new level (e.g. "success") a one-line edit.
const DOT_BG_BY_LEVEL: Record<RowProps["level"], string> = {
  error: "bg-negative",
  warn: "bg-warning",
  info: "bg-fg-faint",
};

function NotificationRow({ level, message, plugin, timestamp, dismissed, onDismiss }: RowProps) {
  return (
    <div
      className={cn(
        "flex items-start gap-2.5 px-3.5 py-2 border-b border-line-soft",
        dismissed && "opacity-50",
      )}
    >
      <div
        className={cn(
          "mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full",
          DOT_BG_BY_LEVEL[level],
        )}
      />
      <div className="min-w-0 flex-1">
        <div className="whitespace-pre-wrap break-words text-[12px] text-fg">{message}</div>
        <div className="mt-0.5 text-[10px] text-fg-faint">
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
