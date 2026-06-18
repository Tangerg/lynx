// PanelTabBar — heterogeneous tabs (chat sessions + workspace views)
// on the left, plugin actions pinned on the right. Single `activeId`;
// when a view tab is active, ChatPanel swaps in its body. Hover ===
// active background; only the 2px accent underline marks the active
// tab.
//
// Layout: the tab strip is its own horizontal scroll viewport; the
// plugin-actions Slot sits OUTSIDE that viewport so the "+" button
// (and any future top-bar actions) stays visible regardless of how
// many tabs are open. Mouse wheel inside the strip is mapped to
// horizontal scroll so a regular mouse can navigate without using
// the (deliberately hidden) scrollbar.

import type { IconName } from "@/components/common";
import type { HeaderTabCloseActions, HeaderTabKind } from "@/state/sessionStore";
import * as ContextMenu from "@radix-ui/react-context-menu";
import { useCallback, useRef } from "react";
import { dragClasses, Icon, noDragClasses, StatusDot, Tooltip } from "@/components/common";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";

export interface ChatTab {
  id: string;
  title: string;
  status: "running" | "waiting" | "idle";
}

export interface ViewTab {
  id: string;
  title: string;
  icon?: string;
}

interface Props {
  tabs: ChatTab[];
  viewTabs: ViewTab[];
  activeId: string;
  onSelectChat: (id: string) => void;
  onSelectView: (id: string) => void;
  onCloseChat: (id: string) => void;
  onCloseView: (id: string) => void;
  /** Resolve the context-menu close actions for the right-clicked
   *  tab. The kind decides which underlying store actions to compose
   *  so the rules treat chat + view tabs as one unified strip. */
  closeActionsFor: (kind: HeaderTabKind, id: string) => HeaderTabCloseActions;
}

export function PanelTabBar({
  tabs,
  viewTabs,
  activeId,
  onSelectChat,
  onSelectView,
  onCloseChat,
  onCloseView,
  closeActionsFor,
}: Props) {
  const stripRef = useRef<HTMLDivElement>(null);
  const onWheel = useCallback((e: React.WheelEvent<HTMLDivElement>) => {
    // Vertical wheel → horizontal scroll so a mouse user can navigate
    // the strip without a visible scrollbar. Trackpad horizontal
    // gestures already work as-is. Only intercept when the strip
    // actually overflows; otherwise let the event bubble so the
    // surrounding panel scroll keeps working.
    const el = stripRef.current;
    if (!el || e.deltaY === 0) return;
    if (el.scrollWidth <= el.clientWidth) return;
    el.scrollLeft += e.deltaY;
    e.preventDefault();
  }, []);

  // Unified-strip positional info. Chat tabs always render before view
  // tabs, so a chat tab can only be the strip's last tab when there
  // are zero view tabs; a view tab can only be the strip's first when
  // there are zero chat tabs.
  const total = tabs.length + viewTabs.length;
  const isOnly = total === 1;

  return (
    <div className={cn("flex min-h-9 items-center gap-1 bg-surface px-4", dragClasses)}>
      <div
        ref={stripRef}
        onWheel={onWheel}
        className="-mb-px flex min-w-0 flex-1 items-end gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {tabs.map((t, i) => (
          <TabItem
            key={`chat:${t.id}`}
            active={t.id === activeId}
            title={t.title}
            leading={<StatusDot tone={t.status} />}
            onSelect={() => onSelectChat(t.id)}
            onClose={() => onCloseChat(t.id)}
            closeActions={closeActionsFor("chat", t.id)}
            isFirst={i === 0}
            isLast={i === tabs.length - 1 && viewTabs.length === 0}
            isOnly={isOnly}
          />
        ))}
        {viewTabs.map((t, i) => (
          <TabItem
            key={`view:${t.id}`}
            active={t.id === activeId}
            title={t.title}
            leading={t.icon ? <Icon name={t.icon as IconName} size={11} /> : null}
            onSelect={() => onSelectView(t.id)}
            onClose={() => onCloseView(t.id)}
            closeActions={closeActionsFor("view", t.id)}
            isFirst={i === 0 && tabs.length === 0}
            isLast={i === viewTabs.length - 1}
            isOnly={isOnly}
          />
        ))}
      </div>
      <Slot name="chat.topbar.actions" />
    </div>
  );
}

interface TabItemProps {
  active: boolean;
  title: string;
  leading: React.ReactNode;
  onSelect: () => void;
  onClose: () => void;
  closeActions: HeaderTabCloseActions;
  isFirst: boolean;
  isLast: boolean;
  isOnly: boolean;
}

function TabItem({
  active,
  title,
  leading,
  onSelect,
  onClose,
  closeActions,
  isFirst,
  isLast,
  isOnly,
}: TabItemProps) {
  const t = useT();
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger asChild>
        <div
          role="tab"
          aria-selected={active}
          tabIndex={0}
          className={cn(
            "group relative inline-grid shrink-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-1.5 rounded-t-md px-3 py-1.5 pr-2 text-[12.5px] text-fg-muted min-w-[110px] max-w-[200px] transition-[color,background-color,transform] duration-150 ease-out focus-visible:outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)] active:scale-[0.98] active:duration-75",
            noDragClasses,
            "hover:bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] hover:text-fg",
            active && [
              "bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] text-fg",
              "after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:bg-accent after:pointer-events-none",
            ],
          )}
          onClick={onSelect}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              onSelect();
            }
          }}
          onAuxClick={(e) => {
            // Middle-click closes — matches every IDE / browser tab
            // convention. button=1 is the wheel button.
            if (e.button === 1) {
              e.preventDefault();
              onClose();
            }
          }}
        >
          {leading}
          {/* View-tab titles are i18n keys (workspace.view.title.*); chat-tab
              titles are session names. t() translates the former, passes the
              latter through. */}
          <span className="truncate font-semibold text-[12.5px] leading-tight" title={t(title)}>
            {t(title)}
          </span>
          <Tooltip label={t("common.close")}>
            <button
              type="button"
              aria-label={t("panel.tab.close")}
              className="grid h-5.5 w-5.5 place-items-center rounded border-0 bg-transparent text-fg-faint opacity-0 transition-all duration-150 group-hover:opacity-100 hover:bg-surface-3 hover:text-fg active:scale-90"
              onClick={(e) => {
                e.stopPropagation();
                onClose();
              }}
            >
              <Icon name="x" size={10} />
            </button>
          </Tooltip>
        </div>
      </ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content
          className="z-50 min-w-[180px] rounded-md border border-line bg-surface p-1 text-[12.5px] text-fg shadow-pop"
          collisionPadding={8}
        >
          <TabMenuItem onSelect={onClose}>{t("common.close")}</TabMenuItem>
          <TabMenuItem disabled={isOnly} onSelect={closeActions.onCloseOthers}>
            {t("panel.tab.closeOthers")}
          </TabMenuItem>
          <TabMenuItem disabled={isFirst} onSelect={closeActions.onCloseLeft}>
            {t("panel.tab.closeLeft")}
          </TabMenuItem>
          <TabMenuItem disabled={isLast} onSelect={closeActions.onCloseRight}>
            {t("panel.tab.closeRight")}
          </TabMenuItem>
          <ContextMenu.Separator className="my-1 h-px bg-line" />
          <TabMenuItem onSelect={closeActions.onCloseAll}>{t("panel.tab.closeAll")}</TabMenuItem>
        </ContextMenu.Content>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  );
}

function TabMenuItem({
  children,
  disabled,
  onSelect,
}: {
  children: React.ReactNode;
  disabled?: boolean;
  onSelect: () => void;
}) {
  return (
    <ContextMenu.Item
      disabled={disabled}
      onSelect={onSelect}
      className="rounded px-2 py-1 outline-none data-[highlighted]:bg-surface-2 data-[disabled]:cursor-not-allowed data-[disabled]:text-fg-faint"
    >
      {children}
    </ContextMenu.Item>
  );
}
