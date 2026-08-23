import type { Plan, PlanStep } from "@lyra/runtime-contract";

export function PlanCompact(props: {
  plan: Plan | undefined;
  pending: boolean;
  error: boolean;
}) {
  const { plan } = props;
  const steps = plan?.steps ?? [];
  const completed = steps.filter((step) => step.status === "completed").length;
  const progress = steps.length === 0 ? 0 : (completed / steps.length) * 100;
  const current =
    steps.find((step) => step.status === "in_progress") ??
    steps.find((step) => step.status === "pending");
  const summary = props.error
    ? "Plan unavailable"
    : props.pending
      ? "Loading plan…"
      : planSummary(steps, current);

  return (
    <div
      className="plan-compact"
      tabIndex={0}
      aria-label={
        props.error
          ? "Current Plan is unavailable"
          : props.pending
          ? "Loading current Plan"
          : `Plan progress: ${completed} of ${steps.length} steps complete. ${summary}`
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
          Plan <b className="tabular">{completed}/{steps.length}</b>
        </span>
        <small>{summary}</small>
      </span>
      <div className="plan-popover" id="plan-details" role="tooltip">
        <header>
          <span>Current plan</span>
          <b className="tabular">rev {plan?.revision ?? 0}</b>
        </header>
        {props.error ? (
          <p>The current Plan could not be read.</p>
        ) : props.pending ? (
          <p aria-busy="true">Loading the current Plan…</p>
        ) : steps.length === 0 ? (
          <p>No plan has been written for this session.</p>
        ) : (
          <ol>
            {steps.map((step) => (
              <li key={step.id} data-status={step.status}>
                <span aria-hidden="true">{statusGlyph(step.status)}</span>
                <span>{step.description}</span>
                <span className="sr-only">{statusLabel(step.status)}</span>
              </li>
            ))}
          </ol>
        )}
      </div>
    </div>
  );
}

function planSummary(steps: PlanStep[], current: PlanStep | undefined): string {
  if (current) return current.description;
  if (steps.length === 0) return "No plan yet";
  return "All steps complete";
}

function statusGlyph(status: string): string {
  if (status === "completed") return "✓";
  if (status === "in_progress") return "●";
  return "○";
}

function statusLabel(status: string): string {
  if (status === "completed") return "Completed";
  if (status === "in_progress") return "In progress";
  if (status === "pending") return "Pending";
  return status;
}
