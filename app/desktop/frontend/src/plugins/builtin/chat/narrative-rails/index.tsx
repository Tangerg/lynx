// The map of the conversation, in the gutter beside the reading column.
//
// It lives beside the scroller rather than inside it so they hold still while
// the transcript moves — which is the only reason a map is useful — and it is a
// contribution rather than shell furniture because a navigation aid over the
// narrative is exactly the kind of thing a plugin should be able to replace.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { TurnRail } from "./ui/TurnRail";

export default definePlugin({
  name: "lyra.builtin.narrative-rails",
  setup(ctx) {
    contributeLayout(ctx, "chat.rail.start", {
      id: "turn-rail",
      order: 0,
      component: TurnRail,
    });
  },
});
