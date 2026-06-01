import { EmptyState, Icon, IconButton } from "@/components/common";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useAgentSlice } from "@/state/agentStore";

function PlanTab() {
  const plan = useAgentSlice((v) => v.plan);
  const done = plan.filter((p) => p.status === "done").length;

  // TODO: pull the goal + ETA from the live agent run once that's
  // surfaced in agentStore. Hard-coded for the design preview.
  const title = "Refactor auth.ts → Result";
  const eta = "est. 2 min remaining";

  return (
    <WorkspaceViewLayout
      icon="list"
      titleStrong
      title={title}
      sub={`${done} of ${plan.length} complete · ${eta}`}
      actions={
        <IconButton title="Edit plan">
          <Icon name="edit" size={14} />
        </IconButton>
      }
    >
      {plan.length === 0 ? (
        <EmptyState
          icon="list"
          title="No plan yet"
          sub="When the agent drafts a plan it shows up here."
        />
      ) : (
        <PlanList plan={plan} />
      )}
    </WorkspaceViewLayout>
  );
}

export const planView = defineWorkspaceView({
  id: "plan",
  title: "Plan",
  icon: "list",
  openByDefault: false,
  order: 30,
  component: PlanTab,
});
