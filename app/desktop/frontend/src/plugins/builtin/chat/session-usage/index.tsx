// The token/cost readout sits in the composer's status strip — `chat.status`, the
// slot that had been declared and left empty while this chip stacked into the
// banner band above the transcript, beside the goal control and the plan progress.
// Those are things you act on; this is a number you glance at, and it belongs next
// to the input whose next turn adds to it.

import { definePlugin } from "@/plugins/sdk";
import { sessionUsageStatusSlot } from "./application/sessionUsageContributions";
import { SessionUsageChip } from "./ui/SessionUsageChip";

export default definePlugin({
  name: "lyra.builtin.session-usage",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.status", sessionUsageStatusSlot(SessionUsageChip));
  },
});
