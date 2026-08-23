import { useState } from "react";

import type { Goal } from "@lyra/runtime-contract";

import type { GoalActions } from "./useGoalActions";

export function GoalTray(props: {
  sessionId: string;
  goal: Goal | undefined;
  pending: boolean;
  error: unknown;
  actions: GoalActions;
}) {
  const [confirmingClear, setConfirmingClear] = useState(false);
  const { goal, actions } = props;

  if (props.error) {
    return (
      <section className="goal-tray goal-tray-empty" role="alert">
        <TrayHeading status="Unavailable" />
        <p>{messageOf(props.error)}</p>
      </section>
    );
  }

  if (props.pending) {
    return (
      <section className="goal-tray goal-tray-empty" aria-busy="true">
        <TrayHeading status="Loading" />
        <p>Reading the current Goal from the session snapshot…</p>
      </section>
    );
  }

  if (!goal) {
    return (
      <section className="goal-tray goal-tray-empty" aria-labelledby="goal-title">
        <TrayHeading status="Not started" />
        <p>Use the Goal composer to give this session a durable objective.</p>
        {actions.error ? (
          <p className="inline-error" role="alert">
            {messageOf(actions.error)}
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
      <TrayHeading status={statusLabel(goal.status)} statusCode={goal.status} />
      <p className="goal-objective">{goal.objective}</p>
      {goal.reason ? (
        <div className="goal-reason">
          <strong>{reasonLabel(goal.reason.code)}</strong>
          {goal.reason.detail ? <p>{goal.reason.detail}</p> : null}
        </div>
      ) : null}
      <div className="goal-usage" aria-label="Goal budget usage">
        <UsageLine
          label="Runs"
          used={goal.used.runs}
          limit={goal.budget.maxRuns}
        />
        <UsageLine
          label="Steps"
          used={goal.used.steps}
          limit={goal.budget.maxSteps}
        />
        <UsageLine
          label="Cost"
          used={goal.used.costUsd}
          limit={goal.budget.maxCostUsd}
          currency
        />
      </div>
      {actions.error ? (
        <p className="inline-error" role="alert">
          {messageOf(actions.error)}
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
            Pause
          </button>
        ) : null}
        {resumable ? (
          <button
            className="primary-action"
            type="button"
            disabled={actions.pending}
            onClick={() => void actions.resume(props.sessionId).catch(() => undefined)}
          >
            Resume
          </button>
        ) : null}
        {confirmingClear ? (
          <div className="clear-confirm" role="group" aria-label="Confirm clear goal">
            <button
              className="quiet-action"
              type="button"
              onClick={() => setConfirmingClear(false)}
            >
              Keep
            </button>
            <button
              className="danger-action"
              type="button"
              disabled={actions.pending}
              onClick={() => void clear()}
            >
              Clear goal
            </button>
          </div>
        ) : (
          <button
            className="quiet-action"
            type="button"
            disabled={actions.pending}
            onClick={() => setConfirmingClear(true)}
          >
            Clear
          </button>
        )}
      </div>
    </section>
  );
}

function TrayHeading(props: { status: string; statusCode?: string }) {
  return (
    <header className="goal-tray-heading">
      <div>
        <span className="eyebrow">Autonomous work</span>
        <h3 id="goal-title">Goal</h3>
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
  const value = props.currency ? formatCost(props.used) : String(props.used);
  const limit =
    props.limit === undefined
      ? undefined
      : props.currency
        ? formatCost(props.limit)
        : String(props.limit);
  return (
    <div>
      <span>
        {props.label}
        <b className="tabular">
          {value}
          {limit ? ` / ${limit}` : ""}
        </b>
      </span>
      {props.limit ? (
        <progress
          value={Math.min(props.used, props.limit)}
          max={props.limit}
          aria-label={`${props.label}: ${value} of ${limit}`}
        />
      ) : null}
    </div>
  );
}

function statusLabel(status: string): string {
  if (status === "active") return "Active";
  if (status === "paused") return "Paused";
  if (status === "blocked") return "Needs attention";
  if (status === "completing") return "Completing";
  return status;
}

function reasonLabel(code: string): string {
  const labels: Record<string, string> = {
    stoppedByUser: "Paused by you",
    runtimeRestarted: "Runtime restarted",
    runStartFailed: "A run could not start",
    awaitingInput: "Waiting for input",
    terminalOutcomeMissing: "Run outcome is unavailable",
    runNotCompleted: "The last run did not complete",
    runBudgetReached: "Run budget reached",
    costBudgetReached: "Cost budget reached",
    stepBudgetReached: "Step budget reached",
    blockedByModel: "The model reported a blocker",
  };
  return labels[code] ?? code;
}

function formatCost(value: number): string {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(value);
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "Goal action failed.";
}
