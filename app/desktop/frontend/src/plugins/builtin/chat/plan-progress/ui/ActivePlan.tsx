import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { StepMark, StepRow } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n";
import { type PlanStep, useSessionPlan } from "@/plugins/builtin/agent/public/plan";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import { activePlanState, type ActivePlanState } from "../application/progress";

export function ActivePlan() {
  const plan = useSessionPlan();
  const progress = activePlanState(plan, useIsCurrentRootRunning());

  return (
    <AnimatePresence initial={false}>
      {progress.visible && progress.current && (
        <PlanDisclosure
          key={plan.identity}
          steps={plan.steps}
          progress={progress}
          current={progress.current}
        />
      )}
    </AnimatePresence>
  );
}

function PlanDisclosure({
  steps,
  progress,
  current,
}: {
  steps: readonly PlanStep[];
  progress: ActivePlanState;
  current: PlanStep;
}) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={disclosureTransition}
      className={expanded ? "w-full" : "max-w-full"}
    >
      <AgentActivityDisclosure
        className={expanded ? "w-full" : "mx-auto w-fit max-w-full"}
        leading={<StepMark state={current.status} />}
        shell="card"
        label={
          <AnimatePresence mode="wait" initial={false}>
            <motion.span
              key={expanded ? "summary" : "current"}
              initial={{ opacity: 0, y: 3 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -3 }}
              transition={disclosureTransition}
              className="block min-w-0 truncate"
            >
              {expanded
                ? t("plan.complete", { done: progress.done, total: progress.total })
                : current.text}
            </motion.span>
          </AnimatePresence>
        }
        // The bar is the row's full bottom edge (see AgentActivityDisclosure) and the
        // count stays in the meta column: one of them is seen while scrolling past,
        // the other is read when the reader stops. Full width keeps progress
        // states visually distinguishable without reading the count.
        progress={{
          value: progress.percent,
          label: t("plan.complete", { done: progress.done, total: progress.total }),
        }}
        trailing={
          <span className="font-mono text-ui-xs font-medium tabular-nums">
            {progress.done}/{progress.total}
          </span>
        }
        open={expanded}
        onToggle={() => setExpanded((value) => !value)}
        toggleLabel={
          expanded
            ? t("plan.collapse")
            : t("plan.expand", {
                done: progress.done,
                total: progress.total,
                pct: progress.percent,
              })
        }
      >
        {/* The same row the Plan panel draws; long steps remain readable here. */}
        <ul className="flex flex-col">
          {steps.map((step) => (
            <li key={step.id}>
              <StepRow state={step.status}>{step.text}</StepRow>
            </li>
          ))}
        </ul>
      </AgentActivityDisclosure>
    </motion.div>
  );
}
