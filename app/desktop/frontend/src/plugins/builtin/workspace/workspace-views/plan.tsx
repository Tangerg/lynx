import { EmptyState } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useActiveRunPlan } from "@/plugins/builtin/agent/public/run";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

function PlanTab() {
  const t = useT();
  const plan = useActiveRunPlan();
  const done = plan.filter((p) => p.status === "done").length;

  return (
    <WorkspaceViewLayout
      icon="list"
      titleStrong
      title="plan.title"
      sub={plan.length ? `${done} of ${plan.length} complete` : undefined}
    >
      {plan.length === 0 ? (
        <EmptyState icon="list" title={t("plan.empty.title")} sub={t("plan.empty.sub")} />
      ) : (
        <PlanList plan={plan} />
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
