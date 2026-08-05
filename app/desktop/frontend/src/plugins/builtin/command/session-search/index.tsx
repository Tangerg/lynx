// Built-in plugin: ⌘K — go to a session by name.
//
// What the command palette left behind. The palette was a third path to things
// that already had a button and a shortcut; this is the one capability that had no
// other home, because the sidebar's session list has no filter and loads every
// session.

import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { useSessionSearchStore } from "../sessionSearchStore";
import {
  sessionSearchOverlaySlot,
  sessionSearchShortcut,
} from "./application/sessionSearchContributions";
import { SessionSearch } from "./ui/SessionSearch";

export default definePlugin({
  name: "lyra.builtin.session-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", sessionSearchOverlaySlot(SessionSearch));
    host.extensions.contribute(
      SHORTCUT,
      sessionSearchShortcut(() => useSessionSearchStore.getState().toggle()),
    );
  },
});
