import type { Plan, PlanStep } from "@lyra/runtime-contract";

import {
  useLocalization,
  type Translate,
} from "../localization/Localization";

export function PlanCompact(props: {
  plan: Plan | undefined;
  pending: boolean;
  error: boolean;
}) {
  const { t } = useLocalization();
  const { plan } = props;
  const steps = plan?.steps ?? [];
  const completed = steps.filter((step) => step.status === "completed").length;
  const progress = steps.length === 0 ? 0 : (completed / steps.length) * 100;
  const current =
    steps.find((step) => step.status === "in_progress") ??
    steps.find((step) => step.status === "pending");
  const summary = props.error
    ? t("plan.unavailable")
    : props.pending
      ? t("plan.loading")
      : planSummary(steps, current, t);

  return (
    <div
      className="plan-compact"
      tabIndex={0}
      aria-label={
        props.error
          ? t("plan.currentUnavailable")
          : props.pending
          ? t("plan.loadingCurrent")
          : t("plan.progress", {
              completed,
              total: steps.length,
              summary,
            })
      }
      aria-describedby="plan-details"
    >
      <svg className="plan-ring" viewBox="0 0 36 36" aria-hidden="true">
        <circle className="plan-ring-track" cx="18" cy="18" r="15.5" />
        <circle
          className="plan-ring-value"
          cx="18"
          cy="18"
          r="15.5"
          pathLength="100"
          strokeDasharray={`${progress} 100`}
        />
      </svg>
      <span className="plan-copy">
        <span>
          {t("plan.title")} <b className="tabular">{completed}/{steps.length}</b>
        </span>
        <small>{summary}</small>
      </span>
      <div className="plan-popover" id="plan-details" role="tooltip">
        <header>
          <span>{t("plan.current")}</span>
          <b className="tabular">
            {t("plan.revision", { revision: plan?.revision ?? 0 })}
          </b>
        </header>
        {props.error ? (
          <p>{t("plan.readFailed")}</p>
        ) : props.pending ? (
          <p aria-busy="true">{t("plan.loadingDetail")}</p>
        ) : steps.length === 0 ? (
          <p>{t("plan.emptyDetail")}</p>
        ) : (
          <ol>
            {steps.map((step) => (
              <li key={step.id} data-status={step.status}>
                <span aria-hidden="true">{statusGlyph(step.status)}</span>
                <span>{step.description}</span>
                <span className="sr-only">{statusLabel(step.status, t)}</span>
              </li>
            ))}
          </ol>
        )}
      </div>
    </div>
  );
}

function planSummary(
  steps: PlanStep[],
  current: PlanStep | undefined,
  t: Translate,
): string {
  if (current) return current.description;
  if (steps.length === 0) return t("plan.empty");
  return t("plan.complete");
}

function statusGlyph(status: string): string {
  if (status === "completed") return "✓";
  if (status === "in_progress") return "●";
  return "○";
}

function statusLabel(status: string, t: Translate): string {
  if (status === "completed") return t("plan.status.completed");
  if (status === "in_progress") return t("plan.status.inProgress");
  if (status === "pending") return t("plan.status.pending");
  return status;
}
