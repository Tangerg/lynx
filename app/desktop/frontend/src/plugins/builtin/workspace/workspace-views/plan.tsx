import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { useCurrentRootPlan } from "@/plugins/builtin/agent/public/run";
import { planSubtext, planViewModel } from "@/plugins/builtin/workspace/application/planViewModel";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

function PlanTab() {
  const t = useT();
  const plan = useCurrentRootPlan();
  const view = planViewModel(plan);

  return (
    <WorkspaceViewLayout icon="list" titleStrong title="plan.title" sub={planSubtext(t, view)}>
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
  order: 120,
  splittable: true,
  component: PlanTab,
});
