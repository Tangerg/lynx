import { definePlugin, lookupExtensionByKey } from "@/plugins/sdk";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { closeActiveWorkspaceView } from "@/plugins/builtin/workspace/public/navigation";
import { usePaletteStore } from "../paletteStore";
import { globalCommandShortcuts, workspaceEscapeShortcut } from "./application/globalKeymap";

export default definePlugin({
  name: "lyra.builtin.global-keymap",
  version: "1.0.0",
  requires: ["lyra.builtin.default-commands", "lyra.builtin.shortcuts"],
  setup({ host }) {
    for (const shortcut of globalCommandShortcuts((id) => lookupExtensionByKey(COMMAND, id))) {
      host.extensions.contribute(SHORTCUT, shortcut);
    }
    host.extensions.contribute(
      SHORTCUT,
      workspaceEscapeShortcut({
        isPaletteOpen: () => usePaletteStore.getState().open,
        closeActiveWorkspaceView,
      }),
    );
  },
});
