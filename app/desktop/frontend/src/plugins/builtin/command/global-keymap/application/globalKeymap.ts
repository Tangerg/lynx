import type { CommandSpec, ShortcutSpec } from "@/plugins/sdk";

export const GLOBAL_COMMAND_IDS = [
  "chat.new",
  "chat.close-session",
  "composer.focus",
  "view.toggle-sidebar",
  "settings.toggle-theme",
];

export type CommandLookup = (id: string) => CommandSpec | undefined;

export interface WorkspaceEscapePorts {
  isPaletteOpen: () => boolean;
  closeActiveWorkspaceView: () => boolean;
}

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

export function handleWorkspaceEscape(ports: WorkspaceEscapePorts): boolean {
  if (ports.isPaletteOpen()) return false;
  return ports.closeActiveWorkspaceView();
}

export function workspaceEscapeShortcut(ports: WorkspaceEscapePorts): ShortcutSpec {
  return {
    key: "Escape",
    description: "Close workspace view",
    allowInInputs: false,
    handler: () => {
      handleWorkspaceEscape(ports);
    },
  };
}
