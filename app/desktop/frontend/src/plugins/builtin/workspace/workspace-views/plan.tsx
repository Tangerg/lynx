import { EmptyState, Icon, IconButton, ScrollArea } from "@/components/common";
import { PlanList } from "@/components/views/PlanList";
import { ViewHeader } from "@/components/views/ViewHeader";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { useAgentSlice } from "@/state/agentStore";

function PlanTab() {
  const plan = useAgentSlice((v) => v.plan);
  const done = plan.filter((p) => p.status === "done").length;

  // TODO: pull the goal + ETA from the live agent run once that's
  // surfaced in agentStore. Hard-coded for the design preview.
  const title = "Refactor auth.ts → Result";
  const eta = "est. 2 min remaining";

  return (
    <>
      <ViewHeader
        icon="list"
        titleStrong
        title={title}
        sub={`${done} of ${plan.length} complete · ${eta}`}
        actions={
          <IconButton title="Edit plan">
            <Icon name="edit" size={14} />
          </IconButton>
        }
      />
      <ScrollArea>
        {plan.length === 0 ? (
          <EmptyState
            icon="list"
            title="No plan yet"
            sub="When the agent drafts a plan it shows up here."
          />
        ) : (
          <PlanList plan={plan} />
        )}
      </ScrollArea>
    </>
  );
}

export const planView = definePlugin({
  name: "lyra.builtin.view-plan",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "plan",
      title: "Plan",
      icon: "list",
      openByDefault: false,
      order: 30,
      component: PlanTab,
    });
  },
});
