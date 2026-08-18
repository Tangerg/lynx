import type { MouseEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { IconButton, StepMark, StepRow } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n";
import { type PlanStep, useSessionPlan } from "@/plugins/builtin/agent/public/plan";
import { planBannerState, type PlanBannerState } from "../application/progress";

export function PlanProgressBanner() {
  const plan = useSessionPlan();
  const [dismissedPlanIdentity, setDismissedPlanIdentity] = useState<string | null>(null);
  const progress = planBannerState(plan, dismissedPlanIdentity);

  const dismiss = (event: MouseEvent) => {
    event.stopPropagation();
    setDismissedPlanIdentity(plan.identity);
  };

  return (
    <AnimatePresence initial={false}>
      {progress.visible && progress.current && (
        <PlanDisclosure
          key={plan.identity}
          steps={plan.steps}
          progress={progress}
          current={progress.current}
          onDismiss={dismiss}
        />
      )}
    </AnimatePresence>
  );
}

function PlanDisclosure({
  steps,
  progress,
  current,
  onDismiss,
}: {
  steps: readonly PlanStep[];
  progress: PlanBannerState;
  current: PlanStep;
  onDismiss: (event: MouseEvent) => void;
}) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={disclosureTransition}
    >
      <AgentActivityDisclosure
        leading={<StepMark state={current.status} />}
        // A banner, not an entry in the transcript: it stands above the stream
        // and has to hold its own edge against whatever scrolls under it.
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
        actions={
          <IconButton
            icon="x"
            iconSize="xs"
            size="sm"
            quiet
            title={t("plan.dismiss")}
            aria-label={t("plan.dismissAria")}
            onClick={onDismiss}
          />
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
