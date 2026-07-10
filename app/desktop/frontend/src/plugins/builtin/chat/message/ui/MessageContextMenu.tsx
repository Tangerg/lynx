// Right-click context menu for a chat message. Mirrors the inline
// `message.header.end` action buttons (Copy / Edit / Regenerate) but
// reaches them via right-click anywhere on the message body — the
// header icons are 16px hover targets, easy to miss; the context menu
// is a Mac/Win-native discovery path users already reach for.
//
// Subscribes to *nothing*: every store read happens inside the
// onSelect handlers via getState(). The component mounts once per
// message in the stream, so subscribing here would mean N selectors
// re-evaluating on every streaming token delta — a real perf cliff
// once history grows past a handful of turns.

import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type { ReactElement, ReactNode } from "react";
import { ContextMenu, Icon } from "@/ui";
import {
  editAndRerunMessage,
  editMessageInComposer,
  forkFromMessage,
  regenerateMessage,
  restoreCheckpoint,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import { messageCopyPayloads } from "@/plugins/builtin/chat/message-actions/public/copyPayloads";
import { messageContextMenuModel } from "@/plugins/builtin/chat/message-actions/public/contextMenu";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { serverFeature } from "@/state/runtimeStore";

interface Props {
  msg: Message;
  children: ReactNode;
}

export function MessageContextMenu({ msg, children }: Props) {
  const t = useT();
  const copy = messageCopyPayloads(msg);
  // Imperative read, not a subscription (see header comment) — capabilities
  // are discovery-time stable and messages can't exist before runtime startup.
  const canRestoreFiles = serverFeature("checkpoints");
  const menu = messageContextMenuModel({ msg, copy, canRestoreFiles });

  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger render={children as ReactElement} />
      <ContextMenu.Content className="min-w-[180px]">
        {menu.copyMarkdown && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(copy.markdown || copy.plain, {
                successLabel: t("msgActions.copiedMarkdown"),
              })
            }
          >
            {t("msgActions.copyMarkdown")}
          </ContextMenu.IconItem>
        )}
        {menu.copyPlain && (
          <ContextMenu.IconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(copy.plain, { successLabel: t("msgActions.copiedPlain") })
            }
          >
            {t("msgActions.copyPlain")}
          </ContextMenu.IconItem>
        )}
        {menu.copyCode && (
          <ContextMenu.IconItem
            icon="code"
            onSelect={() =>
              void writeToClipboard(copy.code, { successLabel: t("msgActions.copiedCode") })
            }
          >
            {t("msgActions.copyCode")}
          </ContextMenu.IconItem>
        )}
        {menu.user.visible && (
          <>
            <Separator />
            {menu.user.editInComposer && (
              <ContextMenu.IconItem icon="edit" onSelect={() => editMessageInComposer(msg)}>
                {t("msgActions.editInComposer")}
              </ContextMenu.IconItem>
            )}
            {/* Destructive variant: rewinds history to before this turn
                  (sessions.rollback), then prefills the composer. */}
            {menu.user.editRerun && (
              <ContextMenu.IconItem icon="loop" onSelect={() => editAndRerunMessage(msg)}>
                {t("msgActions.editRerun")}
              </ContextMenu.IconItem>
            )}
            {/* Same rewind, but also restores the working tree to the
                  pre-turn shadow-git checkpoint (restoreType:"both"). */}
            {menu.user.editRerunRestore && (
              <ContextMenu.IconItem
                icon="history"
                onSelect={() => editAndRerunMessage(msg, { restoreFiles: true })}
              >
                {t("msgActions.editRerunRestore")}
              </ContextMenu.IconItem>
            )}
            {/* Pure restore (no resend, unlike Edit & rerun): rewind to the
                  state BEFORE this turn and stop. Conversation-only always; the
                  file/both variants need the pre-turn shadow-git snapshot. */}
            {menu.user.restore && (
              <ContextMenu.SubmenuRoot>
                <ContextMenu.SubmenuTrigger className="grid-cols-[14px_minmax(0,1fr)_12px]">
                  <Icon name="history" size={12} />
                  <span className="truncate">{t("msgActions.restore")}</span>
                  <Icon name="chevron-down" size={12} className="-rotate-90 text-fg-faint" />
                </ContextMenu.SubmenuTrigger>
                <ContextMenu.Content
                  side="right"
                  align="start"
                  sideOffset={2}
                  alignOffset={-4}
                  className="min-w-[170px]"
                >
                  <ContextMenu.IconItem
                    icon="skip-back"
                    onSelect={() => restoreCheckpoint(msg, "history")}
                  >
                    {t("msgActions.restoreConversation")}
                  </ContextMenu.IconItem>
                  {menu.user.restoreFiles && (
                    <ContextMenu.IconItem
                      icon="folder"
                      onSelect={() => restoreCheckpoint(msg, "files")}
                    >
                      {t("msgActions.restoreFiles")}
                    </ContextMenu.IconItem>
                  )}
                  {menu.user.restoreBoth && (
                    <ContextMenu.IconItem
                      icon="history"
                      onSelect={() => restoreCheckpoint(msg, "both")}
                    >
                      {t("msgActions.restoreBoth")}
                    </ContextMenu.IconItem>
                  )}
                </ContextMenu.Content>
              </ContextMenu.SubmenuRoot>
            )}
            {/* Non-destructive sibling of Edit & rerun: branch a new
                  session that keeps history through this exchange. */}
            {menu.user.fork && (
              <ContextMenu.IconItem icon="branch" onSelect={() => forkFromMessage(msg)}>
                {t("msgActions.fork")}
              </ContextMenu.IconItem>
            )}
          </>
        )}
        {menu.assistant.visible && (
          <>
            <Separator />
            {menu.assistant.regenerate && (
              <ContextMenu.IconItem icon="loop" onSelect={() => regenerateMessage(msg)}>
                {t("msgActions.regenerate")}
              </ContextMenu.IconItem>
            )}
            {menu.assistant.regenerateRestore && (
              <ContextMenu.IconItem
                icon="history"
                onSelect={() => regenerateMessage(msg, { restoreFiles: true })}
              >
                {t("msgActions.regenerateRestore")}
              </ContextMenu.IconItem>
            )}
          </>
        )}
      </ContextMenu.Content>
    </ContextMenu.Root>
  );
}

function Separator() {
  return <ContextMenu.Separator />;
}
