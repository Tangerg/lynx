// Goal mode banner — the run-scoped strip above the composer for the session's
// autonomous objective (goals.*). Renders nothing when the runtime doesn't
// offer goal mode. With the feature on: a slim "set a goal" affordance when
// none is running, or the live drive/stop/resume control (with a budget meter
// that refetches as the loop advances) when one is.

import { useCallback, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { FIELD_CLASSES, Icon, PillButton } from "@/ui";
import { swift } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { notifyError } from "@/lib/notify";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import {
  resumeGoal,
  startGoal,
  stopGoal,
  useGoal,
  useGoalLiveRefetch,
} from "../application/goalConfig";
import type { GoalInfo } from "../application/goalData";

function useAction(): { busy: boolean; run: (op: () => Promise<void>) => void } {
  const t = useT();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const run = useCallback(
    (op: () => Promise<void>) => {
      if (pending.current) return;
      pending.current = true;
      setBusy(true);
      op()
        .catch((err: unknown) => {
          notifyError(err instanceof Error ? err.message : t("goal.error"), { source: "goal" });
        })
        .finally(() => {
          pending.current = false;
          setBusy(false);
        });
    },
    [t],
  );
  return { busy, run };
}

export function GoalBanner() {
  const sessionId = useActiveSessionId();
  const { data } = useGoal(Boolean(sessionId), sessionId ?? undefined);
  const goal = data?.goal ?? null;
  useGoalLiveRefetch(goal?.status === "active");

  if (!sessionId || !data?.available) return null;
  return (
    <AnimatePresence initial={false} mode="wait">
      {goal ? (
        <ActiveGoal key="active" goal={goal} sessionId={sessionId} />
      ) : (
        <StartGoal key="start" sessionId={sessionId} />
      )}
    </AnimatePresence>
  );
}

function statusTone(status: GoalInfo["status"]): string {
  if (status === "active") return "text-accent";
  if (status === "blocked") return "text-negative";
  return "text-fg-muted";
}

function budgetSummary(t: ReturnType<typeof useT>, goal: GoalInfo): string {
  const { budget: b, used: u } = goal;
  const parts: string[] = [];
  parts.push(
    b.maxTurns > 0
      ? `${u.turns}/${b.maxTurns} ${t("goal.turns")}`
      : `${u.turns} ${t("goal.turns")}`,
  );
  if (b.maxCostUsd > 0) parts.push(`$${u.costUsd.toFixed(2)}/$${b.maxCostUsd.toFixed(2)}`);
  else if (u.costUsd > 0) parts.push(`$${u.costUsd.toFixed(2)}`);
  parts.push(
    b.maxSteps > 0
      ? `${u.steps}/${b.maxSteps} ${t("goal.steps")}`
      : `${u.steps} ${t("goal.steps")}`,
  );
  return parts.join(" · ");
}

function ActiveGoal({ goal, sessionId }: { goal: GoalInfo; sessionId: string }) {
  const t = useT();
  const { busy, run } = useAction();
  const driving = goal.status === "active";

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -4 }}
      transition={swift}
      className="mt-2 mb-1 overflow-hidden rounded-lg bg-surface"
    >
      <div className="flex items-start gap-2.5 px-3 py-2.5">
        <Icon name="spark" size={15} className={cn("mt-px shrink-0", statusTone(goal.status))} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-[13px] leading-[1.4] text-fg">{goal.objective}</div>
          <div className="mt-0.5 flex items-center gap-2 text-[11px] text-fg-muted">
            <span className={cn("font-medium", statusTone(goal.status))}>
              {t(`goal.status.${goal.status}`)}
            </span>
            <span className="tabular-nums">{budgetSummary(t, goal)}</span>
          </div>
          {goal.status === "blocked" && goal.reason && (
            <div className="mt-1 text-[11.5px] leading-[1.45] text-fg-soft">{goal.reason}</div>
          )}
        </div>
        {driving ? (
          <PillButton
            size="sm"
            variant="danger"
            disabled={busy}
            onClick={() => run(() => stopGoal(sessionId))}
          >
            {t("goal.stop")}
          </PillButton>
        ) : (
          <PillButton
            size="sm"
            variant="accent"
            disabled={busy}
            onClick={() => run(() => resumeGoal(sessionId))}
          >
            {t("goal.resume")}
          </PillButton>
        )}
      </div>
    </motion.div>
  );
}

function StartGoal({ sessionId }: { sessionId: string }) {
  const t = useT();
  const { busy, run } = useAction();
  const [open, setOpen] = useState(false);
  const [objective, setObjective] = useState("");
  const [maxTurns, setMaxTurns] = useState("");
  const [maxCost, setMaxCost] = useState("");
  const [maxSteps, setMaxSteps] = useState("");
  const canStart = objective.trim() !== "";

  if (!open) {
    return (
      <div className="mt-2 mb-1">
        <button
          type="button"
          onClick={() => setOpen(true)}
          className={cn(
            "flex items-center gap-1.5 rounded-md border-0 bg-transparent px-2 py-1",
            "text-[12px] text-fg-faint transition-colors hover:bg-surface hover:text-fg-soft",
            "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
          )}
        >
          <Icon name="spark" size={13} />
          {t("goal.setGoal")}
        </button>
      </div>
    );
  }

  const start = () => {
    if (!canStart) return;
    const num = (v: string) => {
      const n = Number.parseFloat(v);
      return Number.isFinite(n) && n > 0 ? n : undefined;
    };
    const budget = { maxTurns: num(maxTurns), maxCostUsd: num(maxCost), maxSteps: num(maxSteps) };
    const hasBudget = budget.maxTurns || budget.maxCostUsd || budget.maxSteps;
    run(async () => {
      await startGoal({
        sessionId,
        objective: objective.trim(),
        budget: hasBudget ? budget : undefined,
      });
      setObjective("");
      setMaxTurns("");
      setMaxCost("");
      setMaxSteps("");
      setOpen(false);
    });
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: -4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={swift}
      className="mt-2 mb-1 flex flex-col gap-2 rounded-lg bg-surface p-3"
    >
      <div className="flex items-center gap-1.5 text-[12px] font-medium text-fg-soft">
        <Icon name="spark" size={13} className="text-accent" />
        {t("goal.startTitle")}
      </div>
      <textarea
        aria-label={t("goal.objective")}
        value={objective}
        onChange={(e) => setObjective(e.target.value)}
        placeholder={t("goal.objective.placeholder")}
        spellCheck={false}
        rows={2}
        className={cn(FIELD_CLASSES, "w-full resize-y px-3 py-2 leading-[1.5] text-fg-soft")}
      />
      <div className="grid grid-cols-3 gap-2">
        <BudgetField label={t("goal.maxTurns")} value={maxTurns} onChange={setMaxTurns} />
        <BudgetField label={t("goal.maxCost")} value={maxCost} onChange={setMaxCost} step="0.5" />
        <BudgetField label={t("goal.maxSteps")} value={maxSteps} onChange={setMaxSteps} />
      </div>
      <div className="flex items-center gap-2">
        <PillButton size="sm" variant="accent" disabled={!canStart || busy} onClick={start}>
          {t("goal.start")}
        </PillButton>
        <PillButton size="sm" disabled={busy} onClick={() => setOpen(false)}>
          {t("goal.cancel")}
        </PillButton>
        <span className="ml-auto text-[10.5px] text-fg-faint">{t("goal.budgetHint")}</span>
      </div>
    </motion.div>
  );
}

function BudgetField({
  label,
  value,
  onChange,
  step,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  step?: string;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-[10.5px] text-fg-faint">{label}</span>
      <input
        type="number"
        min={0}
        step={step}
        inputMode="decimal"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="∞"
        className={cn(FIELD_CLASSES, "w-full px-2 py-1 text-[12px] tabular-nums")}
      />
    </label>
  );
}
