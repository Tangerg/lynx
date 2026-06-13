import { EmptyState } from "@/components/common";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useAgentSlice } from "@/state/agentStore";

function PlanTab() {
  const plan = useAgentSlice((v) => v.plan);
  const done = plan.filter((p) => p.status === "done").length;

  return (
    <WorkspaceViewLayout
      icon="list"
      titleStrong
      title="Plan"
      sub={plan.length ? `${done} of ${plan.length} complete` : undefined}
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
  splittable: true,
  component: PlanTab,
});
