// Wires global keyboard shortcuts to palette commands. The combo lives on each
// command (CommandSpec.combo); this plugin only lists WHICH commands get a
// global binding and reads their combo from the catalog — no second copy of the
// combo to drift. The handler resolves the command at trigger time (late
// binding), so a plugin can swap run() without touching the wiring. Cmd+1..9
// stays here as pure tab-switch shortcuts — 9 entries would drown the palette.

import { definePlugin, lookupExtensionByKey } from "@/plugins/sdk";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";

// Commands that get a global key binding; their combo is read from the catalog
// (default-commands registers it), so this is just the membership list.
const GLOBAL_COMMAND_IDS = [
  "chat.new",
  "chat.close-tab",
  "composer.focus",
  "view.toggle-sidebar",
  "settings.toggle-theme",
];

export default definePlugin({
  name: "lyra.builtin.global-keymap",
  version: "1.0.0",
  requires: ["lyra.builtin.default-commands", "lyra.builtin.shortcuts"],
  setup({ host }) {
    for (const id of GLOBAL_COMMAND_IDS) {
      const cmd = lookupExtensionByKey(COMMAND, id);
      if (!cmd?.combo) continue;
      host.extensions.contribute(SHORTCUT, {
        key: cmd.combo,
        description: cmd.label,
        // Cmd+combos must fire from inside the composer too — otherwise
        // ⌘W / ⌘N feel broken whenever the user is typing.
        allowInInputs: true,
        handler: (e) => {
          e.preventDefault();
          void lookupExtensionByKey(COMMAND, id)?.run();
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
