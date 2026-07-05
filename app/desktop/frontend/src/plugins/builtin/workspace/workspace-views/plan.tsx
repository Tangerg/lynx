import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveRunPlan } from "@/plugins/builtin/agent/public/run";
import { planSubtext, planViewModel } from "@/plugins/builtin/workspace/application/planViewModel";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

function PlanTab() {
  const t = useT();
  const plan = useActiveRunPlan();
  const view = planViewModel(plan);

  return (
    <WorkspaceViewLayout icon="list" titleStrong title="plan.title" sub={planSubtext(view)}>
      {view.isEmpty ? (
        <EmptyState icon="list" title={t("plan.empty.title")} sub={t("plan.empty.sub")} />
      ) : (
        <PlanList plan={view.items} />
      )}
    </WorkspaceViewLayout>
  );
}

export const planView = defineWorkspaceView({
  id: "plan",
  title: "workspace.view.title.plan",
  icon: "list",
  order: 30,
  splittable: true,
  component: PlanTab,
});
