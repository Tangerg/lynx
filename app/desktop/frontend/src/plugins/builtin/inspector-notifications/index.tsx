// Built-in plugin: "Notifications" inspector tab — the persistent feed
// behind every `host.notify(...)` call.
//
// The badge counts non-dismissed entries so the icon flags new activity
// even when the tab isn't open.

import { Icon, IconButton, ScrollArea } from "@/components/common";
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
      <div className="insp-head">
        <div className="ficon"><Icon name="chat" size={14} /></div>
        <div style={{ minWidth: 0 }}>
          <div className="ftitle" style={{ fontFamily: "var(--font-ui)", fontSize: 13, fontWeight: 700 }}>
            Notifications
          </div>
          <div className="fsub">{visible.length} unread · {entries.length} total</div>
        </div>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton title="Clear all" onClick={clearAll}>
            <Icon name="x" size={14} />
          </IconButton>
        </div>
      </div>
      <ScrollArea style={{ padding: "4px 0" }}>
        {entries.length === 0 && (
          <div style={{
            padding: "20px 16px",
            color: "var(--color-text-faint)",
            fontSize: 12,
            textAlign: "center",
          }}>
            No notifications yet.
          </div>
        )}
        {entries.map((e) => (
          <div
            key={e.id}
            className={`notification-row ${e.dismissed ? "dismissed" : ""} ${e.level}`}
            style={{
              padding: "8px 14px",
              display: "flex",
              gap: 10,
              alignItems: "flex-start",
              opacity: e.dismissed ? 0.5 : 1,
              borderBottom: "1px solid var(--hairline-2)",
            }}
          >
            <div style={{
              width: 6, height: 6, borderRadius: "50%",
              marginTop: 6,
              background:
                e.level === "error" ? "var(--color-error, #f87171)" :
                e.level === "warn"  ? "var(--color-warn,  #fbbf24)" :
                                      "var(--color-text-faint)",
              flexShrink: 0,
            }} />
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{
                fontSize: 12, color: "var(--color-text)",
                whiteSpace: "pre-wrap", wordBreak: "break-word",
              }}>
                {e.message}
              </div>
              <div style={{
                fontSize: 10, color: "var(--color-text-faint)", marginTop: 3,
              }}>
                {e.plugin} · {timeAgo(e.timestamp)}
              </div>
            </div>
            {!e.dismissed && (
              <IconButton title="Dismiss" onClick={() => dismiss(e.id)}>
                <Icon name="x" size={12} />
              </IconButton>
            )}
          </div>
        ))}
      </ScrollArea>
    </>
  );
}

function useUnreadBadge(): number | undefined {
  const log = useNotificationStore((s) => s.log);
  const n = log.filter((e) => !e.dismissed).length;
  return n > 0 ? n : undefined;
}

export default definePlugin({
  name: "lyra.builtin.inspector-notifications",
  version: "1.0.0",
  setup({ host }) {
    host.inspector.registerTab({
      id: "notifications",
      label: "Notifications",
      icon: "chat",
      order: 50,
      useBadge: useUnreadBadge,
      component: NotificationsTab,
    });
  },
});
