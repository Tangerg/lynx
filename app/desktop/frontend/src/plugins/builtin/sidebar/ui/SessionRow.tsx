import { useState } from "react";
import { ContextMenu, Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/utils";
import type { WorkSession } from "@/plugins/builtin/navigation/public/workIndex";

interface Props {
  session: WorkSession;
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
// One line (Codex reference): icon · title (fills, truncates) · right-aligned
// relative time, with a small live dot replacing the time while the session is
// running (accent) or waiting on input (warning) — accent stays reserved for
// live state, selection is the soft pill. Subtle fg-opacity tints for
// hover/active, fast 75 ms transition.
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
    session.attention === "running"
      ? t("session.status.running")
      : session.attention === "waiting"
        ? t("session.status.waiting")
        : formatRelative(session.time);

  const inner = (
    <>
      <button
        type="button"
        onClick={() => onSelect(session.id)}
        data-chrome-focus=""
        aria-current={active ? "page" : undefined}
        aria-label={session.title}
        className={cn(
          "flex w-full items-center gap-2.5 rounded-md border-0 bg-transparent px-2.5 py-2 text-left transition-[background-color] duration-75 hover:bg-fg/[0.04] focus-visible:bg-fg/[0.055] focus-visible:text-fg focus-visible:outline-none",
          active && "bg-fg/[0.075]",
        )}
      >
        <div
          className={cn(
            "shrink-0 flex items-center justify-center h-4 w-4 transition-colors",
            session.favorite ? "text-accent" : active ? "text-fg" : "text-fg-muted",
          )}
        >
          <Icon name={session.favorite ? "star" : "chat"} size={14} />
        </div>
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
            className="min-w-0 flex-1 rounded-xs border-0 bg-surface-3 px-1 py-0 text-[13px] font-medium leading-[1.5] text-fg outline-none focus-visible:shadow-[inset_0_0_0_1.5px_var(--color-accent)]"
          />
        ) : (
          <>
            <span
              className={cn(
                "min-w-0 flex-1 truncate text-[13px] font-medium leading-[1.3] transition-colors",
                active ? "text-fg" : "text-fg-soft",
              )}
            >
              {session.title}
            </span>
            {session.attention === "none" ? (
              <span
                className="shrink-0 text-[11.5px] leading-none text-fg-faint tabular-nums"
                title={session.time}
              >
                {subText}
              </span>
            ) : (
              // Live states collapse to a single dot (reference "· •") — accent
              // for a running turn, warning when it needs the user. The label
              // stays available on hover.
              <span
                className={cn(
                  "h-1.5 w-1.5 shrink-0 rounded-full",
                  session.attention === "running" ? "bg-accent animate-pulse-dot" : "bg-warning",
                )}
                title={subText}
              />
            )}
          </>
        )}
      </button>
    </>
  );

  const row = <div className="relative group select-none">{inner}</div>;

  if (!onDelete && !onFork && !onRename && !onToggleFavorite) return row;
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger render={row} />
      <ContextMenu.Content className="min-w-[160px]">
        {onToggleFavorite && (
          <ContextMenu.IconItem
            icon="star"
            onSelect={() => onToggleFavorite(session.id, !session.favorite)}
          >
            {session.favorite ? "Unpin" : "Pin to top"}
          </ContextMenu.IconItem>
        )}
        {onRename && (
          <ContextMenu.IconItem icon="edit" onSelect={() => setRenaming(true)}>
            Rename
          </ContextMenu.IconItem>
        )}
        {onFork && (
          <ContextMenu.IconItem icon="branch" onSelect={() => onFork(session.id)}>
            Fork
          </ContextMenu.IconItem>
        )}
        {onDelete && (
          <ContextMenu.IconItem icon="trash" destructive onSelect={() => onDelete(session.id)}>
            Delete
          </ContextMenu.IconItem>
        )}
      </ContextMenu.Content>
    </ContextMenu.Root>
  );
}
