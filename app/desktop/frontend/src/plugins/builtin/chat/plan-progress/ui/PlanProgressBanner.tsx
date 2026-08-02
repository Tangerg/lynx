import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import type { MouseEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useState } from "react";
import { IconButton, StepMark, type StepState } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n";
import { useCurrentRootPlan, useCurrentRootRunId } from "@/plugins/builtin/agent/public/run";
import { planProgress } from "../application/progress";

export function PlanProgressBanner() {
  const t = useT();
  const plan = useCurrentRootPlan();
  const runId = useCurrentRootRunId();
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);
  const progress = planProgress(plan, runId, dismissedRunId);

  useEffect(() => {
    setExpanded(false);
  }, [runId]);

  const dismiss = (event: MouseEvent) => {
    event.stopPropagation();
    setDismissedRunId(runId ?? "");
  };

  return (
    <AnimatePresence initial={false}>
      {progress.visible && progress.current && (
        <motion.div
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={disclosureTransition}
          className="mt-1.5 mb-1"
        >
          <AgentActivityDisclosure
            leading={<StepMark state={STEP_STATE[progress.current.status]} />}
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
                    : progress.current.text}
                </motion.span>
              </AnimatePresence>
            }
            trailing={
              <span className="font-mono text-ui-xs font-medium tabular-nums">
                {progress.percent}%
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
                onClick={dismiss}
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
            <ul className="flex flex-col gap-0.5">
              {plan.map((item) => (
                <li key={item.id} className="flex items-center gap-2 py-0.5">
                  <StepMark state={STEP_STATE[item.status]} />
                  <span className={itemTextClass(item.status)}>{item.text}</span>
                </li>
              ))}
            </ul>
          </AgentActivityDisclosure>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

const STEP_STATE: Record<PlanItem["status"], StepState> = {
  done: "done",
  doing: "active",
  todo: "pending",
};

function itemTextClass(status: PlanItem["status"]) {
  if (status === "done") {
    return "min-w-0 flex-1 truncate text-ui-md leading-body text-fg-faint line-through decoration-line-soft";
  }
  if (status === "doing") {
    return "min-w-0 flex-1 truncate text-ui-md font-semibold leading-body text-fg";
  }
  return "min-w-0 flex-1 truncate text-ui-md leading-body text-fg-soft";
}
