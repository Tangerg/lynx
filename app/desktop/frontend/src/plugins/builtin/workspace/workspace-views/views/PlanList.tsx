import type { PlanStep } from "@/plugins/builtin/agent/public/plan";
import { SectionLabel, StepRow } from "@/ui";
import { useT } from "@/lib/i18n";

// The agent's plan, as a checklist. `PlanStep["status"]` already speaks the row's
// step vocabulary, so there is no per-callsite status table left to keep in sync.
export function PlanList({ steps }: { steps: readonly PlanStep[] }) {
  const t = useT();
  return (
    <div className="px-4.5 py-3.5">
      <SectionLabel className="px-0 pt-0">{t("plan.list.heading")}</SectionLabel>
      {steps.map((step) => (
        <StepRow key={step.id} state={step.status}>
          {step.text}
        </StepRow>
      ))}
    </div>
  );
}
