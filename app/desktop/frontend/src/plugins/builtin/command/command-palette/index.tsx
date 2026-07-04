import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
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

    host.extensions.contribute(SHORTCUT, {
      key: "Mod+K",
      description: "Open the command palette",
      // Cmd+K is the escape hatch users expect while typing in the composer.
      allowInInputs: true,
      handler: (event) => {
        event.preventDefault();
        toggleCommandPalette();
      },
    });

    host.commands.register({
      id: "command.open",
      label: t("command.openPalette"),
      icon: "command",
      group: "General",
      keywords: ["palette", "search", "command"],
      combo: "Mod+K",
      run: openCommandPalette,
    });
  },
});
