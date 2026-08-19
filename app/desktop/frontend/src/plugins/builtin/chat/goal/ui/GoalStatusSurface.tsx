import { AnimatePresence, motion } from "motion/react";
import { useRef, useState } from "react";
import { Icon, IconButton } from "@/ui";
import { disclosureTransition } from "@/lib/motion";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { goalCommandWasRetired, resumeGoal, stopGoal } from "../application/goalCommands";
import { GOAL_STATUS_I18N, goalCanResume } from "../application/goalStatusPresentation";
import { type GoalReadModel, useGoalMaterial } from "../application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";

/** A Goal is a standing instruction, not a dashboard. Codex presents it as one
 * quiet row above the composer: lifecycle, objective, then the exact command
 * available now. Budgets and accounting remain Runtime facts but do not become
 * persistent front-end chrome. */
export function GoalStatusSurface() {
  const material = useGoalMaterial();
  const goal = material.value?.goal;

  return (
    <AnimatePresence initial={false}>
      {goal && (
        <GoalRow
          key={JSON.stringify([goal.sessionId, material.generation, goal.createdAt])}
          goal={goal}
        />
      )}
    </AnimatePresence>
  );
}

function GoalRow({ goal }: { goal: GoalReadModel }) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const commandInFlight = useRef(false);
  const runtimeAvailable = useRuntimeCommandsAvailable();
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
        const fallback = goal.status === "active" ? t("goal.error.pause") : t("goal.error.resume");
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
      data-slot="goal-status-row"
      className="mb-2 flex w-full items-center justify-between gap-2 px-3 py-1.5"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2 text-ui-sm leading-tight">
        <Icon name="target" size="xs" className="shrink-0 text-fg-faint" />
        <span className="shrink-0 text-fg">{t(GOAL_STATUS_I18N[goal.status].label)}</span>
        <span className="min-w-0 truncate text-fg-muted">{goal.objective}</span>
      </div>
      {canChangeStatus && (
        <IconButton
          type="button"
          size="xs"
          icon={goal.status === "active" ? "pause" : "play"}
          quiet
          title={t(goal.status === "active" ? "goal.action.pause" : "goal.action.resume")}
          disabled={busy || !runtimeAvailable}
          aria-busy={busy}
          onClick={() => void changeStatus()}
        />
      )}
    </motion.div>
  );
}
