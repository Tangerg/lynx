// Built-in plugins: per-message action buttons in `message.header.end`.
//
// Three icons rendered in the message header (copy + edit + regenerate).
// Each is its own plugin so a fork that doesn't want one can drop it
// without touching the others. Shared chrome / helpers live in
// _shared.ts.

import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Icon, Tooltip } from "@/components/common";
import {
  flattenCode,
  flattenMarkdown,
  flattenText,
  writeToClipboard,
} from "@/lib/agent/messageContent";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { getCurrentSessionView, useAgentAction } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { ACTION_BTN_CLASSES } from "./_shared";

// ---- Copy: dropdown menu with Markdown / Plain text / Code only. ----
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
          className="z-50 min-w-[160px] overflow-hidden rounded-md border border-line-soft bg-surface p-1 shadow-lg animate-rise-in"
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
      className="flex cursor-pointer flex-col gap-0.5 rounded-sm px-2.5 py-1.5 outline-none data-[highlighted]:bg-surface-2"
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
  const setValue = useComposerStore((s) => s.setValue);
  if (msg.role !== "user") return null;
  const text = flattenText(msg.blocks);
  if (!text) return null;

  const onClick = () => {
    setValue(text);
    // Focus the composer textarea so the user can edit immediately.
    const ta = document.querySelector<HTMLTextAreaElement>(".composer-input");
    ta?.focus();
    ta?.setSelectionRange(text.length, text.length);
  };

  return (
    <Tooltip label="Edit message">
      <button
        type="button"
        onClick={onClick}
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

// ---- Regenerate (assistant messages only): find the preceding user
// prompt and re-send it. AG-UI doesn't have a "fork-from-here" verb, so
// the closest thing we can do is replay that prompt — backend treats it
// as a fresh request and produces a new response. ----

function RegenerateButton() {
  const msg = useCurrentMessage();
  const send = useAgentAction("send");
  if (msg.role !== "assistant") return null;
  if (!send) return null;

  const onClick = () => {
    const { messages } = getCurrentSessionView();
    const idx = messages.findIndex((m) => m.id === msg.id);
    if (idx < 0) return;
    for (let i = idx - 1; i >= 0; i--) {
      const m = messages[i]!;
      if (m.role !== "user") continue;
      const text = flattenText(m.blocks).trim();
      if (text) send(text);
      return;
    }
  };

  return (
    <Tooltip label="Regenerate response">
      <button
        type="button"
        onClick={onClick}
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
