import { useState } from "react";

import type { Goal } from "@lyra/runtime-contract";

import {
  useLocalization,
  type Translate,
} from "../localization/Localization";
import type { GoalActions } from "./useGoalActions";

export function GoalTray(props: {
  sessionId: string;
  goal: Goal | undefined;
  pending: boolean;
  error: unknown;
  actions: GoalActions;
}) {
  const { t } = useLocalization();
  const [confirmingClear, setConfirmingClear] = useState(false);
  const { goal, actions } = props;

  if (props.error) {
    return (
      <section className="goal-tray goal-tray-empty" role="alert">
        <TrayHeading status={t("goal.unavailable")} />
        <p>{messageOf(props.error, t("goal.actionFailed"))}</p>
      </section>
    );
  }

  if (props.pending) {
    return (
      <section className="goal-tray goal-tray-empty" aria-busy="true">
        <TrayHeading status={t("goal.loading")} />
        <p>{t("goal.reading")}</p>
      </section>
    );
  }

  if (!goal) {
    return (
      <section className="goal-tray goal-tray-empty" aria-labelledby="goal-title">
        <TrayHeading status={t("goal.notStarted")} />
        <p>{t("goal.empty")}</p>
        {actions.error ? (
          <p className="inline-error" role="alert">
            {messageOf(actions.error, t("goal.actionFailed"))}
          </p>
        ) : null}
      </section>
    );
  }

  const pauseable = goal.status === "active";
  const resumable = goal.status === "paused" || goal.status === "blocked";
  const clear = async () => {
    try {
      await actions.clear(props.sessionId);
      setConfirmingClear(false);
    } catch {
      // Mutation state is rendered below and remains available for retry.
    }
  };

  return (
    <section className="goal-tray" aria-labelledby="goal-title">
      <TrayHeading
        status={statusLabel(goal.status, t)}
        statusCode={goal.status}
      />
      <p className="goal-objective">{goal.objective}</p>
      {goal.reason ? (
        <div className="goal-reason">
          <strong>{reasonLabel(goal.reason.code, t)}</strong>
          {goal.reason.detail ? <p>{goal.reason.detail}</p> : null}
        </div>
      ) : null}
      <div className="goal-usage" aria-label={t("goal.budgetUsage")}>
        <UsageLine
          label={t("goal.runs")}
          used={goal.used.runs}
          limit={goal.budget.maxRuns}
        />
        <UsageLine
          label={t("goal.steps")}
          used={goal.used.steps}
          limit={goal.budget.maxSteps}
        />
        <UsageLine
          label={t("goal.cost")}
          used={goal.used.costUsd}
          limit={goal.budget.maxCostUsd}
          currency
        />
      </div>
      {actions.error ? (
        <p className="inline-error" role="alert">
          {messageOf(actions.error, t("goal.actionFailed"))}
        </p>
      ) : null}
      <div className="goal-actions">
        {pauseable ? (
          <button
            className="secondary-action"
            type="button"
            disabled={actions.pending}
            onClick={() => void actions.pause(props.sessionId).catch(() => undefined)}
          >
            {t("goal.pause")}
          </button>
        ) : null}
        {resumable ? (
          <button
            className="primary-action"
            type="button"
            disabled={actions.pending}
            onClick={() => void actions.resume(props.sessionId).catch(() => undefined)}
          >
            {t("goal.resume")}
          </button>
        ) : null}
        {confirmingClear ? (
          <div
            className="clear-confirm"
            role="group"
            aria-label={t("goal.confirmClear")}
          >
            <button
              className="quiet-action"
              type="button"
              onClick={() => setConfirmingClear(false)}
            >
              {t("goal.keep")}
            </button>
            <button
              className="danger-action"
              type="button"
              disabled={actions.pending}
              onClick={() => void clear()}
            >
              {t("goal.clearGoal")}
            </button>
          </div>
        ) : (
          <button
            className="quiet-action"
            type="button"
            disabled={actions.pending}
            onClick={() => setConfirmingClear(true)}
          >
            {t("goal.clear")}
          </button>
        )}
      </div>
    </section>
  );
}

function TrayHeading(props: { status: string; statusCode?: string }) {
  const { t } = useLocalization();
  return (
    <header className="goal-tray-heading">
      <div>
        <span className="eyebrow">{t("goal.autonomousWork")}</span>
        <h3 id="goal-title">{t("goal.title")}</h3>
      </div>
      <span className="goal-status" data-status={props.statusCode}>
        {props.status}
      </span>
    </header>
  );
}

function UsageLine(props: {
  label: string;
  used: number;
  limit: number | undefined;
  currency?: boolean;
}) {
  const { formatNumber, locale, t } = useLocalization();
  const limitValue = props.limit;
  const value = props.currency
    ? formatCost(props.used, locale)
    : formatNumber(props.used);
  const limit =
    limitValue === undefined
      ? undefined
      : props.currency
        ? formatCost(limitValue, locale)
        : formatNumber(limitValue);
  return (
    <div>
      <span>
        {props.label}
        <b className="tabular">
          {value}
          {limit ? ` / ${limit}` : ""}
        </b>
      </span>
      {limitValue !== undefined && limit !== undefined ? (
        <progress
          value={Math.min(props.used, limitValue)}
          max={limitValue}
          aria-label={t("goal.usageOf", {
            label: props.label,
            used: value,
            limit,
          })}
        />
      ) : null}
    </div>
  );
}

function statusLabel(status: string, t: Translate): string {
  if (status === "active") return t("goal.status.active");
  if (status === "paused") return t("goal.status.paused");
  if (status === "blocked") return t("goal.status.blocked");
  if (status === "completing") return t("goal.status.completing");
  return status;
}

function reasonLabel(code: string, t: Translate): string {
  const labels: Record<string, string> = {
    stoppedByUser: t("goal.reason.stoppedByUser"),
    runtimeRestarted: t("goal.reason.runtimeRestarted"),
    runStartFailed: t("goal.reason.runStartFailed"),
    awaitingInput: t("goal.reason.awaitingInput"),
    terminalOutcomeMissing: t("goal.reason.terminalOutcomeMissing"),
    runNotCompleted: t("goal.reason.runNotCompleted"),
    runBudgetReached: t("goal.reason.runBudgetReached"),
    costBudgetReached: t("goal.reason.costBudgetReached"),
    stepBudgetReached: t("goal.reason.stepBudgetReached"),
    blockedByModel: t("goal.reason.blockedByModel"),
  };
  return labels[code] ?? code;
}

function formatCost(value: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(value);
}

function messageOf(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}
