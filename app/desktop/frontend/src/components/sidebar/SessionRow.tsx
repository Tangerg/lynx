import type { SidebarSession } from "@/lib/data/queries";
import * as ContextMenu from "@radix-ui/react-context-menu";
import { useState } from "react";
import { Icon, MENU_CONTENT_CLASSES, MenuIconItem, StatusDot } from "@/components/common";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/utils";

interface Props {
  session: SidebarSession;
  active: boolean;
  onSelect: (id: string) => void;
  /** When set, right-click reveals a Rename action (inline title edit). */
  onRename?: (id: string, title: string) => void;
  /** When set, right-click reveals a Fork action (whole-session copy). */
  onFork?: (id: string) => void;
  /** When set, right-click reveals a Delete action. */
  onDelete?: (id: string) => void;
}

// Session row — sidebar list item.
//
// Hover === active background (CLAUDE.md "tab hover === active" rule
// extended to sidebar lists): both states lift to surface-2 + bump
// text to fg. Only the 3px accent indicator bar on the left
// distinguishes "currently selected" from "just hovering" — a single
// visual cue carries the active state, no fighting tone steps.
export function SessionRow({ session, active, onSelect, onRename, onFork, onDelete }: Props) {
  // Inline rename: the context menu flips this on; the title swaps for an
  // input until Enter (commit) or Escape/blur-without-change (cancel).
  const [renaming, setRenaming] = useState(false);
  // `useT()` subscribes to i18next language changes, so the relative
  // time + status labels refresh on locale toggle automatically.
  // formatRelative reads `i18next.t` and `i18next.language` directly
  // — no extra subscription needed.
  const t = useT();
  // Sub-row shows status text when the session is active (Running /
  // Needs input), otherwise the localised time. The previous design
  // had model name here + a separate time column on the right; with
  // titles routinely hitting 25+ chars that right column squeezed
  // the title into ellipsis early. Killing the right column gives
  // the title the full row width.
  const subText =
    session.status === "running"
      ? t("session.status.running")
      : session.status === "waiting"
        ? t("session.status.waiting")
        : formatRelative(session.time);

  const row = (
    <button
      type="button"
      onClick={() => onSelect(session.id)}
      aria-current={active ? "page" : undefined}
      aria-label={session.title}
      className={cn(
        "group relative grid w-full grid-cols-[18px_minmax(0,1fr)] items-center gap-2.5 rounded-lg border-0 bg-transparent px-2.5 py-2 text-left transition-colors duration-150 hover:bg-surface-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
        active && [
          "bg-surface-2",
          "before:content-[''] before:absolute before:-left-1 before:top-2 before:bottom-2 before:w-[3px] before:bg-accent before:rounded-full",
        ],
      )}
    >
      <div
        className={cn(
          "grid h-4.5 w-4.5 place-items-center text-fg-muted transition-colors group-hover:text-fg",
          active && "text-fg",
        )}
      >
        <Icon name="chat" size={14} />
      </div>
      <div className="min-w-0">
        {renaming ? (
          <input
            type="text"
            defaultValue={session.title}
            aria-label="Session title"
            // Rename only ever starts from an explicit user action (the
            // context-menu item), so stealing focus here is the expectation,
            // not a surprise — the a11y concern the rule guards against.
            // eslint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              e.stopPropagation();
              if (e.key === "Escape") setRenaming(false);
              if (e.key === "Enter") {
                const next = e.currentTarget.value.trim();
                if (next && next !== session.title) onRename?.(session.id, next);
                setRenaming(false);
              }
            }}
            onBlur={(e) => {
              const next = e.currentTarget.value.trim();
              if (next && next !== session.title) onRename?.(session.id, next);
              setRenaming(false);
            }}
            className="w-full rounded-xs border-0 bg-surface-3 px-1 py-0 text-[13px] font-semibold leading-[1.5] text-fg outline-none focus-visible:shadow-[inset_0_0_0_1.5px_var(--color-accent)]"
          />
        ) : (
          <div
            className={cn(
              "text-[13px] font-semibold leading-[1.3] truncate transition-colors text-fg-muted group-hover:text-fg",
              active && "text-fg",
            )}
          >
            {session.title}
          </div>
        )}
        <div
          className="mt-0.5 flex items-center gap-1.5 text-[11px] leading-[1.2] text-fg-faint"
          title={session.status === "idle" ? session.time : undefined}
        >
          <StatusDot tone={session.status} />
          <span className="truncate">{subText}</span>
        </div>
      </div>
    </button>
  );

  if (!onDelete && !onFork && !onRename) return row;
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger asChild>{row}</ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content className={cn(MENU_CONTENT_CLASSES, "min-w-[160px]")}>
          {onRename && (
            <MenuIconItem icon="edit" onSelect={() => setRenaming(true)}>
              Rename
            </MenuIconItem>
          )}
          {onFork && (
            <MenuIconItem icon="branch" onSelect={() => onFork(session.id)}>
              Fork
            </MenuIconItem>
          )}
          {onDelete && (
            <MenuIconItem icon="trash" destructive onSelect={() => onDelete(session.id)}>
              Delete
            </MenuIconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  );
}
