// The token/cost readout sits on the chrome bar that names the session it
// counts — `chat.header.meta`. It used to sit in the composer's status strip,
// where it was one more thing between the transcript and the input, and where a
// number that grows during a run pushed the reading column around. On the bar it
// is next to the session title, the run state and the diff stat: the row of
// glanceable facts about the thing you are reading, none of which move anything.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { sessionUsageStatusSlot } from "./application/sessionUsageContributions";
import { SessionUsageChip } from "./ui/SessionUsageChip";

export default definePlugin({
  name: "lyra.builtin.session-usage",
  setup(ctx) {
    contributeLayout(ctx, "chat.header.meta", sessionUsageStatusSlot(SessionUsageChip));
  },
});
