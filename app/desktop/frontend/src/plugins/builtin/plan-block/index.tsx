// Built-in plugin: renderer for the `plan` content block.
//
// The block itself carries no data — it's just a "show the current plan
// here" marker. The renderer pulls the plan from the agent store, so when
// the plan updates later the block re-renders.

import { PlanBlock } from "@/components/chat/PlanBlock";
import { useAgentStore } from "@/state/agentStore";
import { definePlugin } from "@/plugins/sdk";

function PlanContentBlock() {
  const plan = useAgentStore((s) => s.plan);
  return <PlanBlock plan={plan} />;
}

export default definePlugin({
  name: "lyra.builtin.plan-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("plan", PlanContentBlock);
  },
});
