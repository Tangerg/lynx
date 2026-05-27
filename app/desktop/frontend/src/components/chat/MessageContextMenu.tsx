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

import type { Message } from "@/protocol/agui/viewState";
import type { ReactNode } from "react";
import * as ContextMenu from "@radix-ui/react-context-menu";
import { Icon } from "@/components/common";
import { flattenCode, flattenMarkdown, flattenText, writeToClipboard } from "@/lib/messageContent";
import { getCurrentSessionView, useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

interface Props {
  msg: Message;
  children: ReactNode;
}

// Replay the most recent user prompt before the given assistant message.
// Same algorithm as the RegenerateButton in message-actions/index.tsx —
// AG-UI has no "fork-from-here" verb so we resend the last user text.
function regenerate(msg: Message): void {
  const sid = useSessionStore.getState().activeSessionId;
  const send = useAgentStore.getState().sessions[sid]?.send;
  if (!send) return;
  const { messages } = getCurrentSessionView();
  const idx = messages.findIndex((m) => m.id === msg.id);
  if (idx < 0) return;
  for (let i = idx - 1; i >= 0; i--) {
    if (messages[i].role !== "user") continue;
    const text = flattenText(messages[i].blocks).trim();
    if (text) send(text);
    return;
  }
}

// Load the message text back into the composer so the user can tweak
// and re-send. Mirrors the EditButton flow.
function editInComposer(msg: Message): void {
  const text = flattenText(msg.blocks);
  if (!text) return;
  useComposerStore.getState().setValue(text);
  const ta = document.querySelector<HTMLTextAreaElement>(".composer-input");
  ta?.focus();
  ta?.setSelectionRange(text.length, text.length);
}

export function MessageContextMenu({ msg, children }: Props) {
  const markdown = flattenMarkdown(msg.blocks);
  const plain = flattenText(msg.blocks);
  const code = flattenCode(msg.blocks);

  const isUser = msg.role === "user";
  const isAssistant = msg.role === "assistant";
  const canCopy = Boolean(markdown || plain);

  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger asChild>{children}</ContextMenu.Trigger>
      <ContextMenu.Portal>
        <ContextMenu.Content className="z-50 min-w-[180px] overflow-hidden rounded-md border border-line-soft bg-surface p-1 shadow-lg animate-rise-in">
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
              <Item icon="edit" onSelect={() => editInComposer(msg)}>
                Edit in composer
              </Item>
            </>
          )}
          {isAssistant && (
            <>
              <Separator />
              <Item icon="loop" onSelect={() => regenerate(msg)}>
                Regenerate
              </Item>
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
  icon: "copy" | "code" | "edit" | "loop";
  onSelect: () => void;
  children: ReactNode;
}) {
  return (
    <ContextMenu.Item
      onSelect={onSelect}
      className="grid cursor-pointer grid-cols-[14px_minmax(0,1fr)] items-center gap-2 rounded-sm px-2.5 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
    >
      <Icon name={icon} size={12} />
      <span className="truncate">{children}</span>
    </ContextMenu.Item>
  );
}

function Separator() {
  return <ContextMenu.Separator className="mx-1 my-1 h-px bg-line-soft/40" />;
}
