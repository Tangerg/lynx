// How full the model's context window is, on the row you type into.
//
// The composer, not the title bar: a fixed 16px dial does not reflow the control
// row, and its numbers stay in the tooltip where they cost no layout at all.

import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { ContextUsageGauge } from "./ui/ContextUsageGauge";

export default definePlugin({
  name: "scopeapp.builtin.context-usage",
  setup(ctx) {
    contributeLayout(ctx, "composer.toolbar.start", {
      // After the model it measures: the window reads as that control's consequence.
      id: "context-usage",
      order: 3,
      component: ContextUsageGauge,
    });
  },
});
