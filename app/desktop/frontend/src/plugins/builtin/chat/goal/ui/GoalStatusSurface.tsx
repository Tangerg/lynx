import { AnimatePresence, motion } from "motion/react";
import { useRef, useState } from "react";
import { Badge, ProgressBar, TextButton } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { disclosureTransition } from "@/lib/motion";
import { fmtCost } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { formatRelative } from "@/lib/i18n/relativeTime";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { goalCommandWasRetired, resumeGoal, stopGoal } from "../application/goalCommands";
import {
  GOAL_STATUS_I18N,
  GOAL_STOP_I18N,
  goalBudgetAxes,
  goalCanResume,
  tightestAxis,
  type BudgetAxisView,
} from "../application/goalStatusPresentation";
import { type GoalReadModel, useGoalMaterial } from "../application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";

/**
 * The session's standing order, kept in the composer stack.
 *
 * A Goal is authority the user handed over, with an allowance attached. Letting
 * someone hide the readout of how much of that allowance is left would make the
 * loop's remaining reach invisible at exactly the moment it matters.
 */
export function GoalStatusSurface() {
  const material = useGoalMaterial();
  const data = material.value;
  const goal = data?.goal;

  return (
    <AnimatePresence initial={false}>
      {goal && (
        // Objective is editable business content, not incarnation identity: a
        // stopped Goal may be replaced by another with the same words. Runtime
        // does not expose a Goal id, so its immutable Session + creation stamp is
        // the narrowest durable identity available to the read model.
        <GoalDisclosure
          key={JSON.stringify([goal.sessionId, material.generation, goal.createdAt])}
          goal={goal}
        />
      )}
    </AnimatePresence>
  );
}

function GoalDisclosure({ goal }: { goal: GoalReadModel }) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const [busy, setBusy] = useState(false);
  const commandInFlight = useRef(false);
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const axes = goalBudgetAxes(goal);
  const canChangeStatus = goal.status === "active" || goalCanResume(goal);

  const changeStatus = async () => {
    if (commandInFlight.current || !canChangeStatus || !runtimeCommandsAvailable()) return;
    commandInFlight.current = true;
    setBusy(true);
    try {
      if (goal.status === "active") await stopGoal(goal.sessionId);
      else await resumeGoal(goal.sessionId);
    } catch (error) {
      if (!goalCommandWasRetired(error)) {
        const fallback = goal.status === "active" ? t("goal.error.stop") : t("goal.error.resume");
        notifyError(rpcErrorText(error) ?? fallback);
      }
    } finally {
      commandInFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={disclosureTransition}
      className="w-full"
    >
      <AgentActivityDisclosure
        icon="target"
        shell="line"
        tone={GOAL_STATUS_I18N[goal.status].tone}
        label={<span className="block min-w-0 truncate">{goal.objective}</span>}
        trailing={<Allowance axis={tightestAxis(axes)} />}
        // Only when it is not running: "Running" is what an active Goal surface being
        // on screen already says, and a badge repeating it would leave the two
        // states that need saying — paused, blocked — competing with noise.
        actions={
          <div className="flex items-center gap-2">
            {goal.status !== "active" && (
              <Badge tone={GOAL_STATUS_I18N[goal.status].tone}>
                {t(GOAL_STATUS_I18N[goal.status].label)}
              </Badge>
            )}
            {canChangeStatus && (
              <TextButton
                type="button"
                size="sm"
                tone={goal.status === "active" ? "negative" : "accent"}
                disabled={busy || !runtimeAvailable}
                onClick={() => void changeStatus()}
              >
                {goal.status === "active" ? t("goal.action.stop") : t("goal.action.resume")}
              </TextButton>
            )}
          </div>
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
        <GoalMeta goal={goal} />
        {goal.stop && (
          <p className="mt-2.5 text-ui-sm leading-body text-fg-muted">
            {t(GOAL_STOP_I18N[goal.stop.code])}
            {goal.stop.detail && ` — ${goal.stop.detail}`}
          </p>
        )}
      </AgentActivityDisclosure>
    </motion.div>
  );
}

/**
 * The two facts about a standing loop that no budget bar can carry: whether it is
 * still moving, and what it is spending the allowance ON.
 *
 * Both were already in the read model and neither was on screen. "Last move" is
 * the only liveness signal a surface like this can give — an autonomous loop
 * between turns and one that has quietly stopped making progress look identical,
 * and the bars, which only ever climb, cannot tell them apart. The model is here
 * because it is pinned at start: the composer's picker may have moved on, and
 * when there is a cost cap, which model is drawing it down is the whole story.
 *
 * In the expanded body, not the collapsed row: the row is a glance, and it is
 * already carrying the objective, the tightest axis and the status.
 */
function GoalMeta({ goal }: { goal: GoalReadModel }) {
  const t = useT();
  return (
    <dl className="mt-2.5 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-ui-xs text-fg-faint">
      <div className="flex items-baseline gap-1.5">
        <dt>{t("goal.meta.model")}</dt>
        <dd className="m-0 font-mono text-fg-muted">{goal.model}</dd>
      </div>
      <div className="flex items-baseline gap-1.5">
        <dt>{t("goal.meta.lastMove")}</dt>
        <dd className="m-0 text-fg-muted">{formatRelative(goal.updatedAt)}</dd>
      </div>
    </dl>
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
        <ProgressBar value={axis.spent * 100} label={t(axis.label)} className="h-1" />
      )}
      <span className="font-mono text-ui-xs tabular-nums text-fg-muted">
        {axis.spent === undefined
          ? amount(axis, axis.used)
          : `${amount(axis, axis.used)} / ${amount(axis, axis.max)}`}
      </span>
    </div>
  );
}
