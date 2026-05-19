import { Icon, IconButton, ScrollArea } from "@/components/common";
import { PlanInspector } from "@/components/inspector/PlanInspector";
import { useAgentStore } from "@/state/agentStore";
import { definePlugin } from "@/plugins/sdk";

function PlanTab() {
  const plan = useAgentStore((s) => s.plan);
  const done = plan.filter((p) => p.status === "done").length;
  const title = "Refactor auth.ts → Result";
  const eta = "est. 2 min remaining";

  return (
    <>
      <div className="insp-head">
        <div className="ficon"><Icon name="list" size={14} /></div>
        <div style={{ minWidth: 0 }}>
          <div className="ftitle" style={{ fontFamily: "var(--font-ui)", fontSize: 13, fontWeight: 700 }}>
            {title}
          </div>
          <div className="fsub">
            {done} of {plan.length} complete · {eta}
          </div>
        </div>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton title="Edit plan"><Icon name="edit" size={14} /></IconButton>
        </div>
      </div>
      <ScrollArea><PlanInspector plan={plan} /></ScrollArea>
    </>
  );
}

function usePlanBadge(): number | undefined {
  const plan = useAgentStore((s) => s.plan);
  return plan.filter((p) => p.status !== "done").length;
}

export default definePlugin({
  name: "lyra.builtin.inspector-plan",
  version: "1.0.0",
  setup({ host }) {
    host.inspector.registerTab({
      id: "plan",
      label: "Plan",
      icon: "list",
      order: 30,
      useBadge: usePlanBadge,
      component: PlanTab,
    });
  },
});
