// Wires global keyboard shortcuts to palette commands. The combo lives on each
// command (CommandSpec.combo); this plugin only lists WHICH commands get a
// global binding and reads their combo from the catalog — no second copy of the
// combo to drift. The handler resolves the command at trigger time (late
// binding), so a plugin can swap run() without touching the wiring.
//
// Esc closes an open workspace view (Option A — no in-view × button). Fires
// only when: (a) a workspace view is active, (b) the command palette is closed,
// (c) no editable input is focused (allowInInputs: false). This keeps it from
// fighting the composer stop-run binding, the palette dismiss, chat-search
// close, or any Radix overlay.

import { definePlugin, lookupExtensionByKey } from "@/plugins/sdk";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { usePaletteStore } from "@/plugins/builtin/command/command-palette";
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

    // Esc — close the active workspace view and return to chat.
    // Conflict avoidance:
    //   - allowInInputs: false → doesn't fire when composer / any input is
    //     focused (composer has its own Esc → stop-run binding).
    //   - palette state check → doesn't fire while the command palette is open
    //     (palette owns Esc for dismiss).
    //   - Radix dialogs / dropdowns / popovers handle Esc internally and
    //     typically stop propagation; those never reach this handler.
    host.extensions.contribute(SHORTCUT, {
      key: "Escape",
      description: "Close workspace view",
      allowInInputs: false,
      handler: () => {
        if (usePaletteStore.getState().open) return false;
        const { activeMainView, closeMainView } = useSessionStore.getState();
        if (!activeMainView) return false;
        closeMainView(activeMainView);
        return true;
      },
    });
  },
});
