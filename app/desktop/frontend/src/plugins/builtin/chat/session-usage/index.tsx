// The token/cost readout sits on the chrome bar that names the Session it counts.
// It is next to the session title, run state and diff stat: the row of
// glanceable facts about the thing you are reading, none of which move anything.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SessionUsageChip } from "./ui/SessionUsageChip";

export default definePlugin({
  name: "lyra.builtin.session-usage",
  setup(ctx) {
    contributeLayout(ctx, "chat.header.meta", {
      id: "session-usage",
      order: 10,
      component: SessionUsageChip,
    });
  },
});
