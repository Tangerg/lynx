import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { planSubtext, usePlanView } from "@/plugins/builtin/workspace/application/planViewModel";
import { PlanList } from "./views/PlanList";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

// The agent's working plan. Reads live from the explicit Session Plan — the
// backend pushes it via plan.updated, which the fold
// already lands in view.shared. Session-scoped and root-run-written, so it
// outlives the turn that set it.
function PlanTab() {
  const t = useT();
  const view = usePlanView();

  return (
    <WorkspaceViewLayout icon="list" titleStrong title="plan.title" sub={planSubtext(t, view)}>
      {view.state === "unavailable" ? (
        <EmptyState
          icon="list"
          title={t("plan.unavailable.title")}
          sub={t("plan.unavailable.sub")}
        />
      ) : view.state === "empty" ? (
        <EmptyState icon="list" title={t("plan.empty.title")} sub={t("plan.empty.sub")} />
      ) : (
        <PlanList steps={view.steps} />
      )}
    </WorkspaceViewLayout>
  );
}

// Progress through the plan, on the tab. Silent while there is no plan: a tab
// that permanently reads "0/0" trains the eye to stop looking at it.
function PlanTabBadge() {
  const view = usePlanView();
  if (view.total === 0) return null;
  return `${view.done}/${view.total}`;
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
