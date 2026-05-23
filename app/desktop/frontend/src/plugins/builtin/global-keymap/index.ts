// Built-in plugin: wires global keyboard shortcuts to default-commands.
//
// Separation of concerns:
//   - `default-commands` owns the command catalog (palette entries) and
//     the canonical `run()` for each action. The `shortcut` field on a
//     CommandSpec is display-only — it does NOT auto-register a binding.
//   - This plugin reads that catalog and registers actual key bindings
//     via `host.shortcuts.register`. Drop this plugin to disable global
//     shortcuts without losing the palette.
//
// Cmd+1..9 (switch to Nth tab) intentionally do NOT exist as palette
// commands — 9 ungrouped "switch to tab N" entries would drown the
// palette. They live here as pure shortcuts.

import { definePlugin, lookupCommand } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

// (combo, commandId, description) — handler resolves and invokes the
// command at trigger time. Late binding means a plugin can replace the
// command's run() and shortcuts keep working.
const COMMAND_BINDINGS: Array<{
  combo: string;
  commandId: string;
  description: string;
}> = [
  { combo: "Mod+N",       commandId: "chat.new",            description: "New chat tab" },
  { combo: "Mod+W",       commandId: "chat.close-tab",      description: "Close current tab" },
  { combo: "Mod+L",       commandId: "composer.focus",      description: "Focus composer" },
  { combo: "Mod+B",       commandId: "view.toggle-sidebar", description: "Toggle sidebar" },
  { combo: "Mod+Shift+L", commandId: "settings.toggle-theme", description: "Toggle theme" },
];

export default definePlugin({
  name: "lyra.builtin.global-keymap",
  version: "1.0.0",
  requires: ["lyra.builtin.default-commands", "lyra.builtin.shortcuts"],
  setup({ host }) {
    for (const { combo, commandId, description } of COMMAND_BINDINGS) {
      host.shortcuts.register({
        key: combo,
        description,
        // Cmd+combos must fire from inside the composer too — otherwise
        // ⌘W / ⌘N feel broken whenever the user is typing.
        allowInInputs: true,
        handler: (e) => {
          e.preventDefault();
          const cmd = lookupCommand(commandId);
          if (cmd) void cmd.run();
        },
      });
    }

    // Cmd+1..9 — switch to the Nth open tab. Out-of-range numbers no-op
    // silently so the user gets immediate feedback ("nothing happens")
    // without an error popup.
    for (let n = 1; n <= 9; n++) {
      host.shortcuts.register({
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
