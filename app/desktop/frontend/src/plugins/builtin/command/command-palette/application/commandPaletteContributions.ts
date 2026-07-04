import type { CommandSpec, ShortcutSpec } from "@/plugins/sdk";

export type Translate = (key: string) => string;
export type CommandRun = CommandSpec["run"];

export function commandPaletteShortcut(togglePalette: () => void): ShortcutSpec {
  return {
    key: "Mod+K",
    description: "Open the command palette",
    // Cmd+K is the escape hatch users expect while typing in the composer.
    allowInInputs: true,
    handler: (event) => {
      event.preventDefault();
      togglePalette();
    },
  };
}

export function commandPaletteCommand(t: Translate, openPalette: CommandRun): CommandSpec {
  return {
    id: "command.open",
    label: t("command.openPalette"),
    icon: "command",
    group: "General",
    keywords: ["palette", "search", "command"],
    combo: "Mod+K",
    run: openPalette,
  };
}
