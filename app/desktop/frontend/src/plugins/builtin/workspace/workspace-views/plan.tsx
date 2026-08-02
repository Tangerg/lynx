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

// Progress through the plan, on the tab. Silent while there is no plan: a tab
// that permanently reads "0/0" trains the eye to stop looking at it.
function PlanTabBadge() {
  const view = planViewModel(useCurrentRootPlan());
  if (view.isEmpty) return null;
  return `${view.doneCount}/${view.totalCount}`;
}

export const planView = defineWorkspaceView({
  id: "plan",
  title: "workspace.view.title.plan",
  icon: "list",
  badge: PlanTabBadge,
  order: 120,
  splittable: true,
  component: PlanTab,
});
