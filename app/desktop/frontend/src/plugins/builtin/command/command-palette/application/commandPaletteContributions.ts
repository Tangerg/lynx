import type { CommandSpec, LayoutSlotSpec, ShortcutSpec } from "@/plugins/sdk";
import type { Translate } from "@/lib/i18n";

export type CommandRun = CommandSpec["run"];

export function commandPaletteOverlaySlot(component: LayoutSlotSpec["component"]): LayoutSlotSpec {
  return {
    id: "command-palette",
    order: 10,
    component,
  };
}

export function commandPaletteShortcut(t: Translate, togglePalette: () => void): ShortcutSpec {
  return {
    key: "Mod+K",
    description: t("shortcut.commandPalette"),
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
