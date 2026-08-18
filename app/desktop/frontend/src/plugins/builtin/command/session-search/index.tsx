// Built-in plugin: ⌘K — go to a session by name.
//
// What the command palette left behind. The palette was a third path to things
// that already had a button and a shortcut; this is the one capability that had no
// other home, because the sidebar's session list has no filter and loads every
// session.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { sessionSearchLauncher } from "./application/ports/sessionSearchLauncher";
import { installSessionSearchLauncher } from "./adapters/sessionSearchLauncher";
import { SessionSearch } from "./ui/SessionSearch";

export default definePlugin({
  name: "lyra.builtin.session-search",
  setup(ctx) {
    const disposeLauncher = installSessionSearchLauncher();
    contributeLayout(ctx, "app.overlay", {
      id: "session-search",
      order: 10,
      component: SessionSearch,
    });
    ctx.contribute(SHORTCUT, {
      key: "Mod+K",
      description: "shortcut.sessionSearch",
      // Must survive an input having focus so it remains reachable mid-draft.
      allowInInputs: true,
      handler: (event) => {
        event.preventDefault();
        sessionSearchLauncher().toggle();
      },
    });
    ctx.cleanup(disposeLauncher);
  },
});
