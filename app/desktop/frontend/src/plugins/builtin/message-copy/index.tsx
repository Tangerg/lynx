// Built-in plugin: a tiny "copy message" button rendered in the per-
// message actions slot. Demonstrates the message.actions / useCurrentMessage
// pattern.

import { Icon } from "@/components/common";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";

function flattenText(blocks: ReturnType<typeof useCurrentMessage>["blocks"]): string {
  // Best-effort plaintext: only blocks with a `text` field contribute.
  // Tool / approval / search blocks fall through (they don't really make
  // sense to "copy" as plain text anyway).
  return blocks
    .map((b) => ("text" in b ? (b as { text?: string }).text ?? "" : ""))
    .filter(Boolean)
    .join("\n\n");
}

function CopyButton() {
  const msg = useCurrentMessage();
  const text = flattenText(msg.blocks);
  if (!text) return null;

  const onClick = async () => {
    if (typeof navigator === "undefined" || !navigator.clipboard) return;
    try { await navigator.clipboard.writeText(text); } catch { /* unfocused window */ }
  };

  return (
    <button
      className="msg-action-btn"
      onClick={onClick}
      title="Copy message"
      style={{
        background: "transparent",
        border: "none",
        cursor: "pointer",
        color: "var(--color-text-faint)",
        padding: 2,
        display: "inline-flex",
        alignItems: "center",
      }}
    >
      <Icon name="copy" size={11} />
    </button>
  );
}

export default definePlugin({
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
