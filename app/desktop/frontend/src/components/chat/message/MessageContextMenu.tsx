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
import type { ReactNode } from "react";
import * as ContextMenu from "@radix-ui/react-context-menu";
import { Icon, MENU_CONTENT_CLASSES, MENU_ITEM_CLASSES } from "@/components/common";
import {
  editAndRerunMessage,
  editMessageInComposer,
  forkFromMessage,
  regenerateMessage,
} from "@/lib/agent/messageActions";
import {
  flattenCode,
  flattenMarkdown,
  flattenText,
  writeToClipboard,
} from "@/lib/agent/messageContent";
import { serverFeature } from "@/state/runtimeStore";
import { cn } from "@/lib/utils";

interface Props {
  msg: Message;
  children: ReactNode;
}

export function MessageContextMenu({ msg, children }: Props) {
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
      <ContextMenu.Trigger asChild>{children}</ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content className={cn(MENU_CONTENT_CLASSES, "min-w-[180px]")}>
          {canCopy && (
            <Item
              icon="copy"
              onSelect={() =>
                void writeToClipboard(markdown || plain, {
                  successLabel: "Copied as markdown",
                })
              }
            >
              Copy as markdown
            </Item>
          )}
          {plain && (
            <Item
              icon="copy"
              onSelect={() =>
                void writeToClipboard(plain, { successLabel: "Copied as plain text" })
              }
            >
              Copy as plain text
            </Item>
          )}
          {code && (
            <Item
              icon="code"
              onSelect={() => void writeToClipboard(code, { successLabel: "Code copied" })}
            >
              Copy code only
            </Item>
          )}
          {isUser && plain && (
            <>
              <Separator />
              <Item icon="edit" onSelect={() => editMessageInComposer(msg)}>
                Edit in composer
              </Item>
              {/* Destructive variant: rewinds history to before this turn
                  (sessions.rollback), then prefills the composer. */}
              {msg.runId && (
                <Item icon="loop" onSelect={() => editAndRerunMessage(msg)}>
                  Edit & rerun from here
                </Item>
              )}
              {/* Same rewind, but also restores the working tree to the
                  pre-turn shadow-git checkpoint (restoreType:"both"). */}
              {msg.runId && canRestoreFiles && (
                <Item
                  icon="history"
                  onSelect={() => editAndRerunMessage(msg, { restoreFiles: true })}
                >
                  Edit & rerun, restore files
                </Item>
              )}
              {/* Non-destructive sibling of Edit & rerun: branch a new
                  session that keeps history through this exchange. */}
              {msg.runId && (
                <Item icon="branch" onSelect={() => forkFromMessage(msg)}>
                  Fork up to here
                </Item>
              )}
            </>
          )}
          {isAssistant && (
            <>
              <Separator />
              <Item icon="loop" onSelect={() => regenerateMessage(msg)}>
                Regenerate
              </Item>
              {canRestoreFiles && (
                <Item
                  icon="history"
                  onSelect={() => regenerateMessage(msg, { restoreFiles: true })}
                >
                  Regenerate, restore files
                </Item>
              )}
            </>
          )}
        </ContextMenu.Content>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  );
}

function Item({
  icon,
  onSelect,
  children,
}: {
  icon: "copy" | "code" | "edit" | "loop" | "branch" | "history";
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <ContextMenu.Item
      onSelect={onSelect}
      className={cn(MENU_ITEM_CLASSES, "grid-cols-[14px_minmax(0,1fr)]")}
    >
      <Icon name={icon} size={12} />
      <span className="truncate">{children}</span>
    </ContextMenu.Item>
  );
}

function Separator() {
  return <ContextMenu.Separator className="mx-1 my-1 h-px bg-line-soft/40" />;
}
