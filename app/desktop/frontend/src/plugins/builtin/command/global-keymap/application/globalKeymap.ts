import type { CommandSpec, ShortcutSpec } from "@/plugins/sdk";
import type { Translate } from "@/lib/i18n";

// A command's `combo` is only a real shortcut if its id is listed here — this is
// what turns the declaration into a registration.
export const GLOBAL_COMMAND_IDS = [
  "chat.new",
  "chat.search",
  "workspace.close-focused",
  "composer.focus",
  "view.toggle-sidebar",
  "view.toggle-dock",
  "settings.toggle-theme",
  "history.back",
  "history.forward",
];

export type CommandLookup = (id: string) => CommandSpec | undefined;

export function globalCommandShortcuts(lookupCommand: CommandLookup): ShortcutSpec[] {
  return GLOBAL_COMMAND_IDS.flatMap((id) => {
    const command = lookupCommand(id);
    if (!command?.combo) return [];

    return [
      {
        key: command.combo,
        description: command.label,
        allowInInputs: true,
        handler: (event) => {
          event.preventDefault();
          void lookupCommand(id)?.run();
        },
      },
    ];
  });
}

/** Escape has one application meaning: close the active workspace view. */
export function workspaceEscapeShortcut(
  t: Translate,
  closeActiveWorkspaceView: () => boolean,
): ShortcutSpec {
  return {
    key: "Escape",
    description: t("shortcut.closeWorkspaceView"),
    allowInInputs: false,
    handler: () => {
      closeActiveWorkspaceView();
    },
  };
}
