import { useState } from "react";
import { AgentRow } from "@/ui/agent";
import { ConfirmDialog, ContextMenu, TextField } from "@/ui";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/classNames";
import type { WorkSession } from "@/plugins/builtin/navigation/public/workIndex";

interface Props {
  session: WorkSession;
  active: boolean;
  onSelect: (id: string) => void;
  /** When set, right-click reveals a Rename action (inline title edit). */
  onRename?: (id: string, expectedRevision: number, title: string) => void;
  /** When set, right-click reveals a Fork action (whole-session copy). */
  onFork?: (id: string) => void;
  /** When set, right-click reveals a Delete action. */
  onDelete?: (id: string) => void;
  /** When set, right-click reveals a Pin / Unpin action (favorite toggle). */
  onToggleFavorite?: (id: string, expectedRevision: number, favorite: boolean) => void;
}

// Session row — sidebar list item.
//
// One line: icon · title (fills, truncates) · optional live status dot. Idle
// timestamps stay in the accessible label instead of permanently taking the
// row's scarce horizontal space; running/waiting remain visible because they
// require attention now. Accent stays reserved for live state, selection is the
// soft pill.
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
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  // `useT()` subscribes to i18next language changes, so the relative
  // time + status labels refresh on locale toggle automatically.
  // formatRelative reads `i18next.t` and `i18next.language` directly
  // — no extra subscription needed.
  const t = useT();
  const accessibleStatus =
    session.attention === "running"
      ? t("session.status.running")
      : session.attention === "waiting"
        ? t("session.status.waiting")
        : formatRelative(session.time);

  const row = (
    <div className="relative select-none">
      <AgentRow
        icon={session.favorite ? "star" : "chat"}
        iconClassName={session.favorite ? "text-accent" : undefined}
        onClick={() => onSelect(session.id)}
        data-chrome-focus=""
        aria-current={active ? "page" : undefined}
        aria-label={`${session.title} — ${accessibleStatus}`}
        active={active}
        indent="nested"
        className="font-normal"
        trailing={
          renaming || session.attention === "none" ? undefined : (
            <span
              className={cn(
                "h-1.5 w-1.5 shrink-0 rounded-full",
                session.attention === "running" ? "bg-accent animate-pulse-dot" : "bg-warning",
              )}
              title={accessibleStatus}
            />
          )
        }
      >
        {renaming ? (
          <TextField
            variant="bare"
            font="sans"
            defaultValue={session.title}
            aria-label={t("session.row.titleLabel")}
            // Rename only ever starts from an explicit user action (the
            // context-menu item), so stealing focus here is the expectation,
            // not a surprise — the a11y concern the rule guards against.
            // oxlint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return; // let the IME commit its candidate
              e.stopPropagation();
              if (e.key === "Escape") setRenaming(false);
              if (e.key === "Enter") {
                const next = e.currentTarget.value.trim();
                if (next && next !== session.title) {
                  onRename?.(session.id, session.revision, next);
                }
                setRenaming(false);
              }
            }}
            onBlur={(e) => {
              const next = e.currentTarget.value.trim();
              if (next && next !== session.title) {
                onRename?.(session.id, session.revision, next);
              }
              setRenaming(false);
            }}
            className="flex-1 rounded-xs bg-surface-3 px-1 leading-body"
          />
        ) : (
          session.title
        )}
      </AgentRow>
    </div>
  );

  if (!onDelete && !onFork && !onRename && !onToggleFavorite) return row;
  return (
    <>
      <ContextMenu.Root>
        <ContextMenu.Trigger render={row} />
        <ContextMenu.Content className="min-w-[160px]">
          {onToggleFavorite && (
            <ContextMenu.IconItem
              icon="star"
              onSelect={() => onToggleFavorite(session.id, session.revision, !session.favorite)}
            >
              {session.favorite ? t("session.action.unpin") : t("session.action.pin")}
            </ContextMenu.IconItem>
          )}
          {onRename && (
            <ContextMenu.IconItem icon="edit" onSelect={() => setRenaming(true)}>
              {t("session.action.rename")}
            </ContextMenu.IconItem>
          )}
          {onFork && (
            <ContextMenu.IconItem icon="branch" onSelect={() => onFork(session.id)}>
              {t("session.action.fork")}
            </ContextMenu.IconItem>
          )}
          {onDelete && (
            <ContextMenu.IconItem
              icon="trash"
              destructive
              onSelect={() => setConfirmingDelete(true)}
            >
              {t("session.action.delete")}
            </ContextMenu.IconItem>
          )}
        </ContextMenu.Content>
      </ContextMenu.Root>
      {/* Deleting a session is final — the runtime has no restore — so it asks
          first. The menu item used to delete on the way out of the menu. */}
      {onDelete && (
        <ConfirmDialog
          open={confirmingDelete}
          onOpenChange={setConfirmingDelete}
          title={t("session.delete.title")}
          body={t("session.delete.body", { title: session.title })}
          confirmLabel={t("session.action.delete")}
          cancelLabel={t("common.cancel")}
          destructive
          onConfirm={() => onDelete(session.id)}
        />
      )}
    </>
  );
}
