import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import type { MouseEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useState } from "react";
import { Icon, IconButton, StepMark, type StepState } from "@/ui";
import { swift } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useActiveRunId, useActiveRunPlan } from "@/plugins/builtin/agent/public/run";
import { planProgress } from "../application/progress";

export function PlanProgressBanner() {
  const t = useT();
  const plan = useActiveRunPlan();
  const runId = useActiveRunId();
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
          transition={swift}
          className="mt-2 mb-1 overflow-hidden rounded-lg bg-surface"
        >
          <div className="flex items-center">
            <button
              type="button"
              onClick={() => setExpanded((value) => !value)}
              aria-expanded={expanded}
              aria-label={
                expanded
                  ? t("plan.collapse")
                  : t("plan.expand", {
                      done: progress.done,
                      total: progress.total,
                      pct: progress.percent,
                    })
              }
              className={cn(
                "flex min-w-0 flex-1 items-center gap-2.5 px-3 py-2.5",
                "border-0 bg-transparent text-left transition-colors hover:bg-hover",
                "focus-ring-inset",
              )}
            >
              <StepMark state={STEP_STATE[progress.current.status]} />
              <AnimatePresence mode="wait" initial={false}>
                <motion.span
                  key={expanded ? "summary" : "current"}
                  initial={{ opacity: 0, y: 3 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -3 }}
                  transition={swift}
                  className="min-w-0 flex-1 truncate text-ui-lg leading-body text-fg"
                >
                  {expanded
                    ? t("plan.complete", { done: progress.done, total: progress.total })
                    : progress.current.text}
                </motion.span>
              </AnimatePresence>
              <span className="shrink-0 font-mono text-ui-sm font-medium text-fg-muted">
                {progress.percent}%
              </span>
              <Icon
                name={expanded ? "chevron-up" : "chevron-down"}
                size={14}
                className="shrink-0 text-fg-faint"
              />
            </button>
            <IconButton
              icon="x"
              iconSize={12}
              size="sm"
              quiet
              title={t("plan.dismiss")}
              aria-label={t("plan.dismissAria")}
              onClick={dismiss}
              className="mr-1.5 shrink-0"
            />
          </div>

          <div
            className={cn(
              "grid transition-[grid-template-rows] duration-150 ease-out",
              expanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
            )}
          >
            <div className="overflow-hidden">
              <ul className="flex flex-col gap-1 px-3 py-2">
                {plan.map((item) => (
                  <li key={item.id} className="flex items-center gap-2.5 py-0.5">
                    <StepMark state={STEP_STATE[item.status]} />
                    <span className={itemTextClass(item.status)}>{item.text}</span>
                  </li>
                ))}
              </ul>
            </div>
          </div>
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
  return cn(
    "min-w-0 flex-1 truncate text-ui-lg leading-body",
    status === "done" && "text-fg-faint line-through decoration-line-soft",
    status === "doing" && "font-semibold text-fg",
    status === "todo" && "text-fg-soft",
  );
}
