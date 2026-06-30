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

import type { Message } from "@/protocol/run/viewState";
import type { ReactElement, ReactNode } from "react";
import { ContextMenu, Icon, MENU_ITEM_CLASSES, MenuIconItem } from "@/components/common";
import {
  editAndRerunMessage,
  editMessageInComposer,
  forkFromMessage,
  regenerateMessage,
  restoreCheckpoint,
} from "@/lib/agent/messageActions";
import { flattenCode, flattenMarkdown, flattenText } from "@/lib/agent/messageContent";
import { writeToClipboard } from "@/lib/clipboard";
import { useT } from "@/lib/i18n";
import { serverFeature } from "@/state/runtimeStore";
import { cn } from "@/lib/utils";

interface Props {
  msg: Message;
  children: ReactNode;
}

export function MessageContextMenu({ msg, children }: Props) {
  const t = useT();
  const markdown = flattenMarkdown(msg.blocks);
  const plain = flattenText(msg.blocks);
  const code = flattenCode(msg.blocks);

  const isUser = msg.role === "user";
  const isAssistant = msg.role === "assistant";
  const canCopy = Boolean(markdown || plain);
  // Imperative read, not a subscription (see header comment) — capabilities
  // are handshake-time stable and messages can't exist before the handshake.
  const canRestoreFiles = serverFeature("checkpoints");

  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger render={children as ReactElement} />
      <ContextMenu.Content className="min-w-[180px]">
        {canCopy && (
          <MenuIconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(markdown || plain, {
                successLabel: t("msgActions.copiedMarkdown"),
              })
            }
          >
            {t("msgActions.copyMarkdown")}
          </MenuIconItem>
        )}
        {plain && (
          <MenuIconItem
            icon="copy"
            onSelect={() =>
              void writeToClipboard(plain, { successLabel: t("msgActions.copiedPlain") })
            }
          >
            {t("msgActions.copyPlain")}
          </MenuIconItem>
        )}
        {code && (
          <MenuIconItem
            icon="code"
            onSelect={() =>
              void writeToClipboard(code, { successLabel: t("msgActions.copiedCode") })
            }
          >
            {t("msgActions.copyCode")}
          </MenuIconItem>
        )}
        {isUser && plain && (
          <>
            <Separator />
            <MenuIconItem icon="edit" onSelect={() => editMessageInComposer(msg)}>
              {t("msgActions.editInComposer")}
            </MenuIconItem>
            {/* Destructive variant: rewinds history to before this turn
                  (sessions.rollback), then prefills the composer. */}
            {msg.runId && (
              <MenuIconItem icon="loop" onSelect={() => editAndRerunMessage(msg)}>
                {t("msgActions.editRerun")}
              </MenuIconItem>
            )}
            {/* Same rewind, but also restores the working tree to the
                  pre-turn shadow-git checkpoint (restoreType:"both"). */}
            {msg.runId && canRestoreFiles && (
              <MenuIconItem
                icon="history"
                onSelect={() => editAndRerunMessage(msg, { restoreFiles: true })}
              >
                {t("msgActions.editRerunRestore")}
              </MenuIconItem>
            )}
            {/* Pure restore (no resend, unlike Edit & rerun): rewind to the
                  state BEFORE this turn and stop. Conversation-only always; the
                  file/both variants need the pre-turn shadow-git snapshot. */}
            {msg.runId && (
              <ContextMenu.SubmenuRoot>
                <ContextMenu.SubmenuTrigger
                  className={cn(MENU_ITEM_CLASSES, "grid-cols-[14px_minmax(0,1fr)_12px]")}
                >
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
                  <MenuIconItem icon="skip-back" onSelect={() => restoreCheckpoint(msg, "history")}>
                    {t("msgActions.restoreConversation")}
                  </MenuIconItem>
                  {canRestoreFiles && (
                    <MenuIconItem icon="folder" onSelect={() => restoreCheckpoint(msg, "files")}>
                      {t("msgActions.restoreFiles")}
                    </MenuIconItem>
                  )}
                  {canRestoreFiles && (
                    <MenuIconItem icon="history" onSelect={() => restoreCheckpoint(msg, "both")}>
                      {t("msgActions.restoreBoth")}
                    </MenuIconItem>
                  )}
                </ContextMenu.Content>
              </ContextMenu.SubmenuRoot>
            )}
            {/* Non-destructive sibling of Edit & rerun: branch a new
                  session that keeps history through this exchange. */}
            {msg.runId && (
              <MenuIconItem icon="branch" onSelect={() => forkFromMessage(msg)}>
                {t("msgActions.fork")}
              </MenuIconItem>
            )}
          </>
        )}
        {isAssistant && (
          <>
            <Separator />
            <MenuIconItem icon="loop" onSelect={() => regenerateMessage(msg)}>
              {t("msgActions.regenerate")}
            </MenuIconItem>
            {canRestoreFiles && (
              <MenuIconItem
                icon="history"
                onSelect={() => regenerateMessage(msg, { restoreFiles: true })}
              >
                {t("msgActions.regenerateRestore")}
              </MenuIconItem>
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
