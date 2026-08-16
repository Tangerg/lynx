// Built-in plugin: ⌘K — go to a session by name.
//
// What the command palette left behind. The palette was a third path to things
// that already had a button and a shortcut; this is the one capability that had no
// other home, because the sidebar's session list has no filter and loads every
// session.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import {
  sessionSearchOverlaySlot,
  sessionSearchShortcut,
} from "./application/sessionSearchContributions";
import { sessionSearchLauncher } from "./application/ports/sessionSearchLauncher";
import { installSessionSearchLauncher } from "./adapters/sessionSearchLauncher";
import { SessionSearch } from "./ui/SessionSearch";

export default definePlugin({
  name: "lyra.builtin.session-search",
  setup(ctx) {
    const disposeLauncher = installSessionSearchLauncher();
    contributeLayout(ctx, "app.overlay", sessionSearchOverlaySlot(SessionSearch));
    ctx.contribute(
      SHORTCUT,
      sessionSearchShortcut(() => sessionSearchLauncher().toggle()),
    );
    ctx.cleanup(disposeLauncher);
  },
});
