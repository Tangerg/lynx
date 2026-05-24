// ChatTopBar — tabs on the left, plugin-contributed actions on the right.
//
// Tabs are heterogeneous:
//   - chat session tabs (StatusDot + title + close)
//   - workspace-view tabs (icon + title + close) — opened by clicking
//     "↗ Open in main" on a workspace view tab. Owned by `useUIStore.mainViewTabs`.
//
// The single `activeId` decides which one is currently focused. When a
// view tab is active, ChatPanel renders that view's body instead of the
// message stream.

import { Icon, StatusDot, type IconName } from "@/components/common";
import { Slot } from "@/plugins/Slot";

export type ChatTab = {
  id: string;
  title: string;
  status: "running" | "waiting" | "idle";
};

export type ViewTab = {
  id: string;
  title: string;
  icon?: string;
};

type Props = {
  tabs: ChatTab[];
  viewTabs: ViewTab[];
  activeId: string;
  onSelectChat: (id: string) => void;
  onSelectView: (id: string) => void;
  onCloseChat: (id: string) => void;
  onCloseView: (id: string) => void;
};

export function ChatTopBar({
  tabs, viewTabs, activeId,
  onSelectChat, onSelectView, onCloseChat, onCloseView,
}: Props) {
  return (
    <div className="chat-topbar">
      <div className="topbar-tabs">
        {tabs.map((t) => (
          <div
            key={`chat:${t.id}`}
            className={`chat-tab ${t.id === activeId ? "active" : ""}`}
            onClick={() => onSelectChat(t.id)}
          >
            <StatusDot tone={t.status} />
            <span className="tab-title" title={t.title}>{t.title}</span>
            <span
              className="tab-close"
              onClick={(e) => { e.stopPropagation(); onCloseChat(t.id); }}
              title="Close"
            >
              <Icon name="x" size={10} />
            </span>
          </div>
        ))}
        {viewTabs.map((t) => (
          <div
            key={`view:${t.id}`}
            className={`chat-tab view-tab ${t.id === activeId ? "active" : ""}`}
            onClick={() => onSelectView(t.id)}
          >
            {t.icon && <Icon name={t.icon as IconName} size={11} />}
            <span className="tab-title" title={t.title}>{t.title}</span>
            <span
              className="tab-close"
              onClick={(e) => { e.stopPropagation(); onCloseView(t.id); }}
              title="Close"
            >
              <Icon name="x" size={10} />
            </span>
          </div>
        ))}
        <Slot name="chat.topbar.actions" />
      </div>
    </div>
  );
}
