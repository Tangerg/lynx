import { definePlugin, lookupExtensionByKey } from "@/plugins/sdk";
import { COMMAND, SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { closeActiveWorkspaceView } from "@/plugins/builtin/workspace/public/navigation";
import { globalCommandShortcuts, workspaceEscapeShortcut } from "./application/globalKeymap";
import { t } from "@/lib/i18n";

export default definePlugin({
  name: "lyra.builtin.global-keymap",
  setup(ctx) {
    for (const shortcut of globalCommandShortcuts((id) => lookupExtensionByKey(COMMAND, id))) {
      ctx.contribute(SHORTCUT, shortcut);
    }
    ctx.contribute(SHORTCUT, workspaceEscapeShortcut(t, closeActiveWorkspaceView));
  },
});
