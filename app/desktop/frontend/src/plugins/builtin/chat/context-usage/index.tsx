// How full the model's context window is, on the row you type into.
//
// The composer and not the header bar, which is where the session's token/cost
// chip lives and where that chip was deliberately moved TO. The reason it left
// the composer was that a growing number reflowed the row under the reading
// column — a fixed 16px dial cannot, so the objection does not reach this one.
// The numbers stay in its tooltip, where they cost no layout at all.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { composerContextUsageSlot } from "./application/contextUsageContributions";
import { ContextUsageGauge } from "./ui/ContextUsageGauge";

export default definePlugin({
  name: "lyra.builtin.context-usage",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", composerContextUsageSlot(ContextUsageGauge));
  },
});
