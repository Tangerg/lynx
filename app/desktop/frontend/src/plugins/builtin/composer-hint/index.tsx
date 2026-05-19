// Built-in plugin: the "⌘K commands · ⌘↵ send" hint rendered on the right
// side of the composer toolbar. Pluginized so a re-skinned build can
// remove or replace the text without forking Composer.

import { definePlugin } from "@/plugins/sdk";

function KeyHint() {
  return (
    <div className="meta">
      <span className="accent">⌘K</span> commands · <span className="accent">⌘↵</span> send
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.composer-hint",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.end", {
      id: "kbd-hint",
      order: 0,
      component: KeyHint,
    });
  },
});
