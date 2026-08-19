import { AnimatePresence, motion } from "motion/react";
import { Gauge, Pressable, RichTooltip, StepRow } from "@/ui";
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
        <PlanPill
          key={plan.identity}
          steps={plan.steps}
          progress={progress}
          current={progress.current}
        />
      )}
    </AnimatePresence>
  );
}

/** Codex keeps Plan progress as a fixed-size reading aid above the composer. The
 * complete checklist is secondary material and appears only on hover/focus. */
function PlanPill({
  steps,
  progress,
  current,
}: {
  steps: readonly PlanStep[];
  progress: ActivePlanState;
  current: PlanStep;
}) {
  const t = useT();
  const currentIndex = Math.max(
    0,
    steps.findIndex((step) => step.id === current.id),
  );
  const progressLabel = t("plan.progress", {
    current: currentIndex + 1,
    total: progress.total,
  });
  const completionLabel = t("plan.complete", {
    done: progress.done,
    total: progress.total,
  });

  const trigger = (
    <Pressable
      type="button"
      data-slot="active-plan-pill"
      aria-label={progressLabel}
      className="-my-1.5 inline-flex max-w-full min-w-0 items-center gap-1.5 rounded-sm py-1.5 text-ui-sm text-fg-muted transition-colors duration-[var(--dur-color)] hover:text-fg"
    >
      <Gauge value={progress.percent / 100} label={completionLabel} className="text-accent" />
      <span className="truncate whitespace-nowrap tabular-nums">{progressLabel}</span>
    </Pressable>
  );

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={disclosureTransition}
      className="mb-2 flex max-w-full justify-center"
    >
      <RichTooltip
        trigger={trigger}
        side="top"
        sideOffset={8}
        delay={0}
        className="max-h-[min(320px,calc(100vh-16px))] w-[min(320px,calc(100vw-16px))] overflow-y-auto bg-fg px-2 py-2 text-on-fg"
      >
        <ul className="flex flex-col gap-1">
          {steps.map((step) => (
            <li key={step.id}>
              <StepRow state={step.status} className="items-start py-1 font-normal text-on-fg">
                {step.text}
              </StepRow>
            </li>
          ))}
        </ul>
      </RichTooltip>
    </motion.div>
  );
}
