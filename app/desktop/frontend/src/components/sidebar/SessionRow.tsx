import type { SidebarSession } from "@/lib/data/queries";
import * as ContextMenu from "@radix-ui/react-context-menu";
import { useState } from "react";
import { Icon, MENU_CONTENT_CLASSES, MenuIconItem } from "@/components/common";
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
  /** When set, right-click reveals a Pin / Unpin action (favorite toggle). */
  onToggleFavorite?: (id: string, favorite: boolean) => void;
}

// Session row — sidebar list item.
//
// Craft-aligned: subtle foreground-opacity tints for hover/active (2–3%),
// fast 75 ms transition, 8 px radius, 2 px accent indicator bar. The
// outer wrapper carries the selection bar so it sits flush at the left
// edge; the button is a flex row with a leading icon + content column.
export function SessionRow({
  session,
  active,
  onSelect,
  onRename,
  onFork,
  onDelete,
  onToggleFavorite,
}: Props) {
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

  const inner = (
    <>
      {active && <div className="absolute left-0 inset-y-0 w-[2px] bg-accent rounded-full" />}
      <button
        type="button"
        onClick={() => onSelect(session.id)}
        aria-current={active ? "page" : undefined}
        aria-label={session.title}
        className={cn(
          "flex w-full items-start gap-2.5 rounded-md border-0 bg-transparent px-3 py-2 text-left transition-[background-color] duration-75 hover:bg-fg/[0.02] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
          active && "bg-fg/[0.03]",
        )}
      >
        <div
          className={cn(
            "shrink-0 flex items-center justify-center h-4.5 w-4.5 transition-colors",
            session.favorite ? "text-accent" : "text-fg-muted group-hover:text-fg",
            active && !session.favorite && "text-fg",
          )}
        >
          <Icon name={session.favorite ? "star" : "chat"} size={14} />
        </div>
        <div className="flex flex-col gap-1 min-w-0 flex-1">
          {renaming ? (
            <input
              type="text"
              defaultValue={session.title}
              aria-label={t("session.row.titleLabel")}
              // Rename only ever starts from an explicit user action (the
              // context-menu item), so stealing focus here is the expectation,
              // not a surprise — the a11y concern the rule guards against.
              // eslint-disable-next-line jsx-a11y/no-autofocus
              autoFocus
              onClick={(e) => e.stopPropagation()}
              onKeyDown={(e) => {
                if (e.nativeEvent.isComposing) return; // let the IME commit its candidate
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
              className="w-full rounded-xs border-0 bg-surface-3 px-1 py-0 text-[13px] font-medium leading-[1.5] text-fg outline-none focus-visible:shadow-[inset_0_0_0_1.5px_var(--color-accent)]"
            />
          ) : (
            <div
              className={cn(
                "text-[13px] font-medium leading-[1.3] truncate transition-colors text-fg-muted group-hover:text-fg",
                active && "text-fg",
              )}
            >
              {session.title}
            </div>
          )}
          <div
            className="text-[12px] leading-[1.2] text-fg-faint"
            title={session.status === "idle" ? session.time : undefined}
          >
            {subText}
          </div>
        </div>
      </button>
    </>
  );

  const row = <div className="relative group select-none pl-2">{inner}</div>;

  if (!onDelete && !onFork && !onRename && !onToggleFavorite) return row;
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger asChild>{row}</ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content className={cn(MENU_CONTENT_CLASSES, "min-w-[160px]")}>
          {onToggleFavorite && (
            <MenuIconItem
              icon="star"
              onSelect={() => onToggleFavorite(session.id, !session.favorite)}
            >
              {session.favorite ? "Unpin" : "Pin to top"}
            </MenuIconItem>
          )}
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
