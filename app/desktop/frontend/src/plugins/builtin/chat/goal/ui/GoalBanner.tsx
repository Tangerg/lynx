import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { Badge, ProgressBar } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { disclosureTransition } from "@/lib/motion";
import { fmtCost } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import {
  GOAL_STATUS_I18N,
  GOAL_STOP_I18N,
  goalBudgetAxes,
  tightestAxis,
  type BudgetAxisView,
} from "../application/goalBanner";
import { useGoal } from "../application/goalQueries";

/**
 * The session's standing order, pinned above the stream.
 *
 * NOT dismissible, unlike the plan banner beside it. A plan is the agent's own
 * advice about the order it will do things in; a Goal is authority the user
 * handed over, with an allowance attached. Letting someone hide the readout of
 * how much of that allowance is left would make the loop's remaining reach
 * invisible at exactly the moment it matters.
 */
export function GoalBanner() {
  const t = useT();
  const sessionId = useActiveSessionId();
  const { data } = useGoal(sessionId ? { sessionId } : undefined);
  const [expanded, setExpanded] = useState(false);
  const goal = data?.goal;
  const axes = goal ? goalBudgetAxes(goal) : [];

  return (
    <AnimatePresence initial={false}>
      {goal && (
        <motion.div
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={disclosureTransition}
          className="mt-1.5 mb-1"
        >
          <AgentActivityDisclosure
            icon="loop"
            shell="card"
            tone={GOAL_STATUS_I18N[goal.status].tone}
            label={<span className="block min-w-0 truncate">{goal.objective}</span>}
            trailing={<Allowance axis={tightestAxis(axes)} />}
            // Only when it is not running: "Running" is what a goal banner being
            // on screen already says, and a badge repeating it would leave the two
            // states that need saying — paused, blocked — competing with noise.
            actions={
              goal.status !== "active" && (
                <Badge tone={GOAL_STATUS_I18N[goal.status].tone}>
                  {t(GOAL_STATUS_I18N[goal.status].label)}
                </Badge>
              )
            }
            open={expanded}
            onToggle={() => setExpanded((value) => !value)}
            toggleLabel={expanded ? t("goal.collapse") : t("goal.expand")}
          >
            <div className="flex flex-col gap-1.5">
              {axes.map((axis) => (
                <BudgetAxis key={axis.label} axis={axis} />
              ))}
            </div>
            {goal.stop && (
              <p className="mt-2.5 text-ui-sm leading-body text-fg-muted">
                {t(GOAL_STOP_I18N[goal.stop.code])}
                {goal.stop.detail && ` — ${goal.stop.detail}`}
              </p>
            )}
          </AgentActivityDisclosure>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

/** A number in the axis's own unit — dollars are written as money, everything
 *  else as the count it is. */
function amount(axis: BudgetAxisView, value: number): string {
  return axis.unit === "cost" ? fmtCost(value) : String(value);
}

/** The collapsed row's one number: how far into the tightest allowance it is. */
function Allowance({ axis }: { axis: BudgetAxisView | undefined }) {
  const t = useT();
  if (!axis) return <span className="text-ui-xs text-fg-faint">{t("goal.budget.uncapped")}</span>;
  return (
    <span className="font-mono text-ui-xs font-medium tabular-nums">
      {amount(axis, axis.used)}/{amount(axis, axis.max)}
    </span>
  );
}

function BudgetAxis({ axis }: { axis: BudgetAxisView }) {
  const t = useT();
  return (
    <div className="grid grid-cols-[4.5rem_minmax(0,1fr)_auto] items-center gap-2.5">
      <span className="text-ui-sm text-fg-faint">{t(axis.label)}</span>
      {axis.spent === undefined ? (
        <span className="text-ui-sm text-fg-faint">{t("goal.budget.uncapped")}</span>
      ) : (
        <ProgressBar value={axis.spent * 100} className="h-1" />
      )}
      <span className="font-mono text-ui-xs tabular-nums text-fg-muted">
        {axis.spent === undefined
          ? amount(axis, axis.used)
          : `${amount(axis, axis.used)} / ${amount(axis, axis.max)}`}
      </span>
    </div>
  );
}
