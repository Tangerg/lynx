// Built-in plugin: AG-UI CUSTOM event handlers for the run plan.
//
//   "lyra.plan"        → replace state.plan with the snapshot
//   "lyra.plan-block"  → append a `plan` content block to a message
//
// These two used to be a switch case inside `reducer.onCustom`. Pulling them
// into a plugin keeps the reducer agnostic and proves the registry can handle
// real-world handlers.

import { appendBlockToMessage, definePlugin, setPlan } from "@/plugins/sdk";
import { CUSTOM, type PlanBlockAttachment, type PlanSnapshot } from "@/protocol/agui/customEvents";

export default definePlugin({
  name: "lyra.builtin.plan-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<PlanSnapshot>(CUSTOM.PLAN, (value) => setPlan(value.items));

    host.agui.on<PlanBlockAttachment>(CUSTOM.PLAN_BLOCK, (value) =>
      appendBlockToMessage(value.messageId, { kind: "plan" }),
    );
  },
});
