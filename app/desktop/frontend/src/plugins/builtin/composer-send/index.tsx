// Built-in plugin: the round "send" button at the trailing edge of the
// composer toolbar. Used to live as a hardcoded JSX inside Composer.tsx;
// pulling it out demonstrates that even the most "fundamental" UI bit can
// be a plugin contribution.
//
// Wires the same `submitComposer` helper the textarea Enter handler uses
// (registered by `composer-keymap`), so slash commands work identically
// from click and keypress.

import { Icon } from "@/components/common";
import { submitComposer } from "@/components/chat/submitComposer";
import { definePlugin } from "@/plugins/sdk";
import { useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";

function SendButton() {
  const value = useComposerStore((s) => s.value);
  const clear = useComposerStore((s) => s.clear);
  const send = useAgentStore((s) => s.send);

  const disabled = !value.trim() || !send;
  const onClick = () => {
    if (!send) return;
    submitComposer({ value, clear, sendText: send });
  };

  return (
    <button
      className="send-btn"
      disabled={disabled}
      onClick={onClick}
      title="Send (⌘↵)"
    >
      <Icon name="send-arrow" size={14} strokeWidth={2.5} />
    </button>
  );
}

export default definePlugin({
  name: "lyra.builtin.composer-send",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.end", {
      // Order chosen so the send button sits after the kbd hint (order 0).
      id: "send",
      order: 100,
      component: SendButton,
    });
  },
});
