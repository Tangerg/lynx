// Built-in plugin: the toolbar items at the *start* of the composer footer
// row — model picker + attach-file button. These live before the mode
// toggles (which are themselves plugin contributions via composer-modes).
//
// Both items are still placeholder UI for now (clicking the model picker
// shows nothing; the paperclip does nothing), but pulling them out of the
// shell means a "real model selector" plugin or a Claude-Files attach
// plugin can swap them in without forking Composer.tsx.

import { Icon, type IconName } from "@/components/common";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function ModelPicker() {
  const { data: sessions = [] } = useSessions();
  const activeId = useUIStore((s) => s.activeSessionId);
  const active = sessions.find((s) => s.id === activeId) ?? sessions[0];
  const model = active?.model ?? "Sonnet";

  return (
    <button className="composer-model" title="Switch model">
      <span className="cm-avatar">{model.slice(0, 1)}</span>
      <span className="cm-name">{model}</span>
      <Icon name="more" size={10} />
    </button>
  );
}

function AttachButton() {
  return (
    <button className="composer-tool-btn" title="Attach file">
      <Icon name={"paperclip" as IconName} size={13} />
    </button>
  );
}

export default definePlugin({
  name: "lyra.builtin.composer-toolbar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.start", {
      id: "model",  order: 0, component: ModelPicker,
    });
    host.layout.register("composer.toolbar.start", {
      id: "attach", order: 1, component: AttachButton,
    });
  },
});
