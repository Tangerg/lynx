// Built-in plugin: contributes the three default composer modes (agent /
// ask / plan). Other plugins can `host.composer.registerMode(...)` more
// (e.g. a "research" or "code-review" mode that tweaks the system prompt).

import { definePlugin } from "@/plugins/sdk";

export default definePlugin({
  name: "lyra.builtin.composer-modes",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerMode({ id: "agent", label: "Agent", icon: "spark", order: 0 });
    host.composer.registerMode({ id: "ask",   label: "Ask",   icon: "chat",  order: 1 });
    host.composer.registerMode({ id: "plan",  label: "Plan",  icon: "list",  order: 2 });
  },
});
