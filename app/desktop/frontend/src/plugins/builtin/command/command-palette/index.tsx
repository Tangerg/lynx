import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import {
  commandPaletteCommand,
  commandPaletteShortcut,
} from "./application/commandPaletteContributions";
import { openCommandPalette, toggleCommandPalette } from "./application/paletteActions";
import { CommandPalette } from "./ui/CommandPalette";

export default definePlugin({
  name: "lyra.builtin.command-palette",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "command-palette",
      order: 10,
      component: CommandPalette,
    });

    host.extensions.contribute(SHORTCUT, commandPaletteShortcut(toggleCommandPalette));

    host.commands.register(commandPaletteCommand(t, openCommandPalette));
  },
});
