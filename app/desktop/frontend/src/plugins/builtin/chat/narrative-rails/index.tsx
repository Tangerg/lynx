// The two navigation rails that flank the reading column.
//
// They live beside the scroller rather than inside it so they hold still while
// the transcript moves — which is the only reason a map is useful — and they are
// contributions rather than shell furniture because navigation aids over the
// narrative are exactly the kind of thing a plugin should be able to replace.

import { definePlugin } from "@/plugins/sdk";
import { messageOutlineRailSlot, turnRailSlot } from "./application/narrativeRailContributions";
import { MessageOutlineRail } from "./ui/MessageOutlineRail";
import { TurnRail } from "./ui/TurnRail";

export default definePlugin({
  name: "lyra.builtin.narrative-rails",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.rail.start", turnRailSlot(TurnRail));
    host.layout.register("chat.rail.end", messageOutlineRailSlot(MessageOutlineRail));
  },
});
