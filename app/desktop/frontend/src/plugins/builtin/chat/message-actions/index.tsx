// Built-in plugins: per-message action buttons in `message.header.end`.
//
// Three icons rendered in the message header (copy + edit + regenerate).
// Each is its own plugin so a fork that doesn't want one can drop it
// without touching the others. Shared chrome / helpers live in
// _shared.ts.

import { useState } from "react";
import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Icon, MENU_CONTENT_CLASSES, Tooltip } from "@/components/common";
import { editMessageInComposer, regenerateMessage } from "@/lib/agent/messageActions";
import { flattenCode, flattenMarkdown, flattenText } from "@/lib/agent/messageContent";
import { writeToClipboard } from "@/lib/clipboard";
import { cn } from "@/lib/utils";
import { getContainer } from "@/main/container";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { asItemId, asRunId, asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";
import { ACTION_BTN_CLASSES } from "./_shared";

//
// Default click writes Markdown (preserves headings / lists / fences
// as they were rendered). The submenu surfaces the two alternates:
// Plain text drops markup so it pastes flat into editors, Code only
// concatenates the fenced code blocks for users who just want the
// generated snippets. Code variant hides when the message has none.

function CopyButton() {
  const msg = useCurrentMessage();
  const markdown = flattenMarkdown(msg.blocks);
  const plain = flattenText(msg.blocks);
  const code = flattenCode(msg.blocks);
  if (!markdown && !plain) return null;

  return (
    <DropdownMenu.Root>
      <Tooltip label="Copy message">
        <DropdownMenu.Trigger aria-label="Copy message" className={ACTION_BTN_CLASSES}>
          <Icon name="copy" size={11} />
        </DropdownMenu.Trigger>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="end"
          sideOffset={4}
          className={cn(MENU_CONTENT_CLASSES, "min-w-[160px]")}
        >
          <CopyItem
            label="Copy markdown"
            hint="Headings / fences kept"
            onSelect={() => writeToClipboard(markdown, { successLabel: "Copied as markdown" })}
          />
          <CopyItem
            label="Copy plain text"
            hint="Markup stripped"
            onSelect={() => writeToClipboard(plain, { successLabel: "Copied as plain text" })}
          />
          {code && (
            <CopyItem
              label="Copy code only"
              hint="Fenced blocks joined"
              onSelect={() => writeToClipboard(code, { successLabel: "Code copied" })}
            />
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function CopyItem({
  label,
  hint,
  onSelect,
}: {
  label: string;
  hint: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenu.Item
      onSelect={onSelect}
      className="flex flex-col gap-0.5 rounded-sm px-2.5 py-1.5 outline-none data-[highlighted]:bg-surface-2"
    >
      <span className="text-[12.5px] text-fg">{label}</span>
      <span className="text-[11px] text-fg-faint">{hint}</span>
    </DropdownMenu.Item>
  );
}

export const messageCopy = definePlugin({
  name: "lyra.builtin.message-copy",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.header.end", {
      id: "copy",
      order: 0,
      component: CopyButton,
    });
  },
});

// ---- Edit (user messages only): load the text back into the composer
// so the user can tweak and re-send. Doesn't mutate the original message
// — sending creates a new user turn. ----

function EditButton() {
  const msg = useCurrentMessage();
  if (msg.role !== "user") return null;
  if (!flattenText(msg.blocks)) return null;

  return (
    <Tooltip label="Edit message">
      <button
        type="button"
        onClick={() => editMessageInComposer(msg)}
        aria-label="Edit message"
        className={ACTION_BTN_CLASSES}
      >
        <Icon name="edit" size={11} />
      </button>
    </Tooltip>
  );
}

export const messageEdit = definePlugin({
  name: "lyra.builtin.message-edit",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.header.end", {
      id: "edit",
      order: 5,
      component: EditButton,
    });
  },
});

// ---- Regenerate (assistant messages only): replay the preceding user
// prompt via the shared regenerateMessage action (lib/agent). ----

function RegenerateButton() {
  const msg = useCurrentMessage();
  if (msg.role !== "assistant") return null;

  return (
    <Tooltip label="Regenerate response">
      <button
        type="button"
        onClick={() => regenerateMessage(msg)}
        aria-label="Regenerate response"
        className={ACTION_BTN_CLASSES}
      >
        <Icon name="loop" size={11} />
      </button>
    </Tooltip>
  );
}

export const messageRegenerate = definePlugin({
  name: "lyra.builtin.message-regenerate",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.header.end", {
      id: "regenerate",
      order: 10,
      component: RegenerateButton,
    });
  },
});

// ---- Feedback (assistant messages only): thumbs up / down wired to
// `feedback.create`. The wire is write-only (no read-back API), so the
// settled rating lives in a session-lifetime map — same scope as the
// approval "remember" decisions. Re-rating re-submits; the runtime
// treats each as a new event. ----

const ratedMessages = new Map<string, "positive" | "negative">();

function FeedbackButtons() {
  const msg = useCurrentMessage();
  const [rated, setRated] = useState(() => ratedMessages.get(msg.id));
  if (msg.role !== "assistant") return null;

  const rate = (rating: "positive" | "negative"): void => {
    if (rated === rating) return;
    ratedMessages.set(msg.id, rating);
    setRated(rating);
    const sessionId = useSessionStore.getState().activeSessionId;
    // Fire-and-forget: a lost feedback event isn't worth an error banner,
    // but roll the latch back so the user can retry.
    getContainer()
      .client()
      .feedback.create({
        sessionId: sessionId ? asSessionId(sessionId) : undefined,
        runId: msg.runId ? asRunId(msg.runId) : undefined,
        itemId: asItemId(msg.id),
        rating,
      })
      .catch((err: unknown) => {
        console.warn("[feedback] create failed:", err);
        ratedMessages.delete(msg.id);
        setRated(undefined);
      });
  };

  return (
    <>
      <Tooltip label="Good response">
        <button
          type="button"
          onClick={() => rate("positive")}
          aria-label="Good response"
          aria-pressed={rated === "positive"}
          className={cn(ACTION_BTN_CLASSES, rated === "positive" && "text-success")}
        >
          <Icon name="thumbs-up" size={11} />
        </button>
      </Tooltip>
      <Tooltip label="Poor response">
        <button
          type="button"
          onClick={() => rate("negative")}
          aria-label="Poor response"
          aria-pressed={rated === "negative"}
          className={cn(ACTION_BTN_CLASSES, rated === "negative" && "text-negative")}
        >
          <Icon name="thumbs-down" size={11} />
        </button>
      </Tooltip>
    </>
  );
}

export const messageFeedback = definePlugin({
  name: "lyra.builtin.message-feedback",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.header.end", {
      id: "feedback",
      order: 15,
      component: FeedbackButtons,
    });
  },
});
