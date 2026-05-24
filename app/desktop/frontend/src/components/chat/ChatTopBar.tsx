// ChatTopBar — tabs on the left, plugin-contributed actions on the right.
//
// Tabs are heterogeneous:
//   - chat session tabs (StatusDot + title + close)
//   - workspace-view tabs (icon + title + close) — opened by clicking
//     "↗ Open in main" on a workspace view tab. Owned by
//     `useSessionStore.mainViewTabs`.
//
// The single `activeId` decides which one is currently focused. When a
// view tab is active, ChatPanel renders that view's body instead of the
// message stream.
//
// The hover and active states share the same background (per design
// rule "tab hover === active"); only the bottom 2px accent underline
// distinguishes the active tab. Wails drag region covers the whole
// strip — interactive children opt out via `[--wails-draggable:no-drag]`.

import { Icon, StatusDot, type IconName } from "@/components/common";
import { cn } from "@/lib/utils";
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

// Shared tab pill — drives hover background, active accent underline,
// interactive opt-out from the Wails drag region.
const tabClass = (active: boolean) =>
  cn(
    "group relative inline-grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-1.5 rounded-t-md px-3 py-1.5 pr-2 text-[12.5px] text-fg-muted cursor-pointer min-w-[110px] max-w-[200px] transition-colors duration-150 ease-out",
    "[-webkit-app-region:no-drag] [--wails-draggable:no-drag]",
    "hover:bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] hover:text-fg",
    active && [
      "bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] text-fg",
      "after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:bg-accent after:pointer-events-none",
    ],
  );

export function ChatTopBar({
  tabs,
  viewTabs,
  activeId,
  onSelectChat,
  onSelectView,
  onCloseChat,
  onCloseView,
}: Props) {
  return (
    <div className="flex min-h-9 items-center gap-1 bg-surface px-4 [-webkit-app-region:drag] [--wails-draggable:drag]">
      <div className="-mb-px flex min-w-0 flex-1 items-end gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        {tabs.map((t) => (
          <div
            key={`chat:${t.id}`}
            className={tabClass(t.id === activeId)}
            onClick={() => onSelectChat(t.id)}
          >
            <StatusDot tone={t.status} />
            <span className="truncate font-semibold text-[12.5px] leading-tight" title={t.title}>
              {t.title}
            </span>
            <span
              className="grid h-5.5 w-5.5 place-items-center rounded text-fg-faint opacity-0 transition-all duration-150 group-hover:opacity-100 hover:bg-surface-3 hover:text-fg active:scale-90"
              onClick={(e) => {
                e.stopPropagation();
                onCloseChat(t.id);
              }}
              title="Close"
            >
              <Icon name="x" size={10} />
            </span>
          </div>
        ))}
        {viewTabs.map((t) => (
          <div
            key={`view:${t.id}`}
            className={tabClass(t.id === activeId)}
            onClick={() => onSelectView(t.id)}
          >
            {t.icon && <Icon name={t.icon as IconName} size={11} />}
            <span className="truncate font-semibold text-[12.5px] leading-tight" title={t.title}>
              {t.title}
            </span>
            <span
              className="grid h-5.5 w-5.5 place-items-center rounded text-fg-faint opacity-0 transition-all duration-150 group-hover:opacity-100 hover:bg-surface-3 hover:text-fg active:scale-90"
              onClick={(e) => {
                e.stopPropagation();
                onCloseView(t.id);
              }}
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
