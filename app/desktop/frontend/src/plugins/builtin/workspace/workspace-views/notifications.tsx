// Built-in plugin: "Notifications" workspace view — the persistent feed
// behind every `host.notify(...)` call.

import { useMemo } from "react";
import { EmptyState, Icon, IconButton, StatusDot } from "@/ui";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/utils";
import { useNotificationStore } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { defineWorkspaceView } from "./defineWorkspaceView";

function NotificationsTab() {
  const t = useT();
  const log = useNotificationStore((s) => s.log);
  const dismiss = useNotificationStore((s) => s.dismiss);
  const clearAll = useNotificationStore((s) => s.clearAll);

  // Newest first; memoized so a re-render that isn't a log change doesn't re-copy.
  const entries = useMemo(() => [...log].reverse(), [log]);
  const visible = useMemo(() => entries.filter((e) => !e.dismissed), [entries]);

  return (
    <WorkspaceViewLayout
      icon="chat"
      titleStrong
      title="notifications.title"
      sub={`${visible.length} unread · ${entries.length} total`}
      scrollClassName="py-1"
      actions={
        <IconButton title={t("notifications.clearAll")} onClick={clearAll}>
          <Icon name="x" size={14} />
        </IconButton>
      }
    >
      {entries.length === 0 && (
        <EmptyState
          icon="chat"
          title={t("notifications.empty.title")}
          sub={t("notifications.empty.sub")}
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
    </WorkspaceViewLayout>
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

// Level → StatusDot tone. Lookup table beats a nested ternary and makes
// adding a new level (e.g. "success") a one-line edit.
const DOT_TONE_BY_LEVEL: Record<RowProps["level"], "err" | "waiting" | "idle"> = {
  error: "err",
  warn: "waiting",
  info: "idle",
};

function NotificationRow({ level, message, plugin, timestamp, dismissed, onDismiss }: RowProps) {
  const t = useT();
  return (
    <div className={cn("flex items-start gap-2.5 px-3.5 py-2", dismissed && "opacity-50")}>
      <StatusDot tone={DOT_TONE_BY_LEVEL[level]} className="mt-1.5" />
      <div className="min-w-0 flex-1">
        <div className="whitespace-pre-wrap break-words text-[13px] text-fg-soft">{message}</div>
        <div className="mt-0.5 text-[11px] text-fg-muted">
          {plugin} · {formatRelative(timestamp)}
        </div>
      </div>
      {!dismissed && (
        <IconButton title={t("notifications.dismiss")} onClick={onDismiss}>
          <Icon name="x" size={12} />
        </IconButton>
      )}
    </div>
  );
}

export const notificationsView = defineWorkspaceView({
  id: "notifications",
  title: "workspace.view.title.notifications",
  icon: "chat",
  order: 50,
  component: NotificationsTab,
});
