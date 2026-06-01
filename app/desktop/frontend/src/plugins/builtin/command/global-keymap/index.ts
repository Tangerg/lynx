// Wires global keyboard shortcuts to palette commands. CommandSpec's
// `shortcut` field is display-only; this plugin reads the catalog and
// registers the bindings. Cmd+1..9 stays here as pure shortcuts —
// 9 "switch to tab N" entries would drown the palette.

import { definePlugin, lookupExtensionByKey } from "@/plugins/sdk";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";

// Late binding: the handler resolves the command at trigger time, so
// a plugin can replace its run() without rewriting the key wiring.
const COMMAND_BINDINGS: Array<{
  combo: string;
  commandId: string;
  description: string;
}> = [
  { combo: "Mod+N", commandId: "chat.new", description: "New chat tab" },
  { combo: "Mod+W", commandId: "chat.close-tab", description: "Close current tab" },
  { combo: "Mod+L", commandId: "composer.focus", description: "Focus composer" },
  { combo: "Mod+B", commandId: "view.toggle-sidebar", description: "Toggle sidebar" },
  { combo: "Mod+Shift+L", commandId: "settings.toggle-theme", description: "Toggle theme" },
];

export default definePlugin({
  name: "lyra.builtin.global-keymap",
  version: "1.0.0",
  requires: ["lyra.builtin.default-commands", "lyra.builtin.shortcuts"],
  setup({ host }) {
    for (const { combo, commandId, description } of COMMAND_BINDINGS) {
      host.extensions.contribute(SHORTCUT, {
        key: combo,
        description,
        // Cmd+combos must fire from inside the composer too — otherwise
        // ⌘W / ⌘N feel broken whenever the user is typing.
        allowInInputs: true,
        handler: (e) => {
          e.preventDefault();
          const cmd = lookupExtensionByKey(COMMAND, commandId);
          if (cmd) void cmd.run();
        },
      });
    }

    // Cmd+1..9 — switch to the Nth open tab. Out-of-range numbers no-op
    // silently so the user gets immediate feedback ("nothing happens")
    // without an error popup.
    for (let n = 1; n <= 9; n++) {
      host.extensions.contribute(SHORTCUT, {
        key: `Mod+${n}`,
        description: `Switch to tab ${n}`,
        allowInInputs: true,
        handler: (e) => {
          e.preventDefault();
          const { tabIds, selectTab } = useSessionStore.getState();
          const target = tabIds[n - 1];
          if (target) selectTab(target);
        },
      });
    }
  },
});
