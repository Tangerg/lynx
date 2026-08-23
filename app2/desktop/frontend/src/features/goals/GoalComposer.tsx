import { useEffect, useReducer, useState, type FormEvent } from "react";

import type { Goal, GoalBudget } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
import type { GoalActions } from "./useGoalActions";

interface GoalComposerProps {
  sessionId: string;
  goal: Goal | undefined;
  actions: GoalActions;
}

interface ObjectiveDraft {
  value: string;
  canonical: string;
  conflicted: boolean;
}

type ObjectiveDraftAction =
  | { type: "input"; value: string }
  | { type: "canonical"; value: string };

export function GoalComposer({ sessionId, goal, actions }: GoalComposerProps) {
  const { t } = useLocalization();
  const canonicalObjective = goal?.objective ?? "";
  const [draft, dispatch] = useReducer(objectiveDraftReducer, {
    value: canonicalObjective,
    canonical: canonicalObjective,
    conflicted: false,
  });
  const [maxRuns, setMaxRuns] = useState("");
  const [maxSteps, setMaxSteps] = useState("");
  const [maxCost, setMaxCost] = useState("");

  useEffect(() => {
    dispatch({ type: "canonical", value: canonicalObjective });
  }, [canonicalObjective]);

  const objective = draft.value.trim();
  const dirty = objective !== draft.canonical;
  const canonicalMoved = draft.conflicted;

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (objective === "" || actions.pending) return;
    try {
      if (goal) {
        if (objective !== goal.objective) {
          await actions.update({ sessionId, objective });
        }
      } else {
        await actions.start({
          sessionId,
          objective,
          budget: goalBudget(maxRuns, maxSteps, maxCost),
        });
      }
    } catch {
      // The mutation owns and presents the error; the draft remains untouched.
    }
  };

  return (
    <form className="goal-composer" onSubmit={submit}>
      <div className="goal-mode-label">
        <span>/goal</span>
        <div>
          <strong>
            {goal ? t("goal.editObjective") : t("goal.newObjective")}
          </strong>
          <small>{t("goal.runtimeOwnership")}</small>
        </div>
      </div>
      <label className="sr-only" htmlFor="goal-objective">
        {t("goal.objective")}
      </label>
      <textarea
        id="goal-objective"
        value={draft.value}
        onChange={(event) =>
          dispatch({ type: "input", value: event.currentTarget.value })
        }
        placeholder={t("goal.objectivePlaceholder")}
        rows={4}
        maxLength={8_000}
        disabled={actions.pending}
      />
      {canonicalMoved ? (
        <p className="draft-notice" role="status">
          {t("goal.externalChange")}
        </p>
      ) : null}
      {!goal ? (
        <details className="goal-budget-fields">
          <summary>{t("goal.optionalBudget")}</summary>
          <div>
            <NumberField
              label={t("goal.runs")}
              value={maxRuns}
              onChange={setMaxRuns}
              step="1"
            />
            <NumberField
              label={t("goal.steps")}
              value={maxSteps}
              onChange={setMaxSteps}
              step="1"
            />
            <NumberField
              label={t("goal.costUSD")}
              value={maxCost}
              onChange={setMaxCost}
              step="0.01"
            />
          </div>
        </details>
      ) : null}
      <footer>
        <span>
          {goal
            ? dirty
              ? t("goal.unsaved")
              : t("goal.upToDate")
            : t("goal.durableStart")}
        </span>
        <button
          className="primary-action"
          type="submit"
          disabled={
            actions.pending || objective === "" || (goal !== undefined && !dirty)
          }
        >
          {actions.pending
            ? t("goal.saving")
            : goal
              ? t("goal.update")
              : t("goal.start")}
        </button>
      </footer>
    </form>
  );
}

function NumberField(props: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  step: string;
}) {
  const { t } = useLocalization();
  return (
    <label>
      <span>{props.label}</span>
      <input
        type="number"
        min="0"
        step={props.step}
        value={props.value}
        onChange={(event) => props.onChange(event.currentTarget.value)}
        placeholder={t("goal.unlimited")}
      />
    </label>
  );
}

function goalBudget(
  maxRuns: string,
  maxSteps: string,
  maxCost: string,
): GoalBudget | undefined {
  const budget: GoalBudget = {};
  const runs = positiveInteger(maxRuns);
  const steps = positiveInteger(maxSteps);
  const cost = positiveNumber(maxCost);
  if (runs !== undefined) budget.maxRuns = runs;
  if (steps !== undefined) budget.maxSteps = steps;
  if (cost !== undefined) budget.maxCostUsd = cost;
  return Object.keys(budget).length === 0 ? undefined : budget;
}

function positiveInteger(value: string): number | undefined {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function positiveNumber(value: string): number | undefined {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function objectiveDraftReducer(
  state: ObjectiveDraft,
  action: ObjectiveDraftAction,
): ObjectiveDraft {
  if (action.type === "input") {
    return {
      ...state,
      value: action.value,
      conflicted:
        state.conflicted && action.value.trim() !== state.canonical,
    };
  }
  const wasClean = state.value.trim() === state.canonical;
  return {
    canonical: action.value,
    value: wasClean ? action.value : state.value,
    conflicted: !wasClean && state.value.trim() !== action.value,
  };
}
