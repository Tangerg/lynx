// Built-in plugins: per-message action buttons in `message.header.end`.
//
// Three icons rendered in the message header (copy + edit + regenerate).
// Each is its own plugin so a fork that doesn't want one can drop it
// without touching the others. Shared chrome / helpers live in
// _shared.ts.

import { Icon } from "@/components/common";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { getCurrentSessionView, useAgentAction } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { ACTION_BTN_CLASSES, flattenText } from "./_shared";

// ---- Copy: plaintext flatten of the message into the clipboard. ----

function CopyButton() {
  const msg = useCurrentMessage();
  const text = flattenText(msg.blocks);
  if (!text) return null;

  const onClick = async () => {
    if (typeof navigator === "undefined" || !navigator.clipboard) return;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      /* unfocused window — silent */
    }
  };

  return (
    <button
      type="button"
      onClick={onClick}
      title="Copy message"
      aria-label="Copy message"
      className={ACTION_BTN_CLASSES}
    >
      <Icon name="copy" size={11} />
    </button>
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
    <button
      type="button"
      onClick={onClick}
      title="Edit message"
      aria-label="Edit message"
      className={ACTION_BTN_CLASSES}
    >
      <Icon name="edit" size={11} />
    </button>
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
      if (messages[i].role !== "user") continue;
      const text = flattenText(messages[i].blocks).trim();
      if (text) send(text);
      return;
    }
  };

  return (
    <button
      type="button"
      onClick={onClick}
      title="Regenerate response"
      aria-label="Regenerate response"
      className={ACTION_BTN_CLASSES}
    >
      <Icon name="loop" size={11} />
    </button>
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
