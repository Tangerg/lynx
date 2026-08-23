import type { Interrupt } from "@lyra/runtime-contract";

import {
  createApprovalDraft,
  type ApprovalDraft,
  type InterruptDraft,
} from "./interruptResponse";

interface ApprovalInterruptProps {
  interrupt: Interrupt;
  index: number;
  draft: InterruptDraft;
  disabled: boolean;
  onChange(update: (draft: InterruptDraft) => InterruptDraft): void;
}

export function ApprovalInterrupt(props: ApprovalInterruptProps) {
  const approval = props.draft.approval ?? createApprovalDraft(props.interrupt);
  const tool = props.interrupt.payload?.tool;
  const update = (patch: Partial<ApprovalDraft>) => {
    props.onChange((draft) => ({
      ...draft,
      approval: { ...approval, ...patch },
    }));
  };

  return (
    <section className="interrupt-request approval-request">
      <div className="request-title">
        <span>{props.index + 1}</span>
        <div>
          <small>Approval</small>
          <h4>{tool?.name ?? "Tool execution"}</h4>
        </div>
        {props.interrupt.payload?.risk ? (
          <span
            className="risk-badge"
            data-risk={props.interrupt.payload.risk}
          >
            {props.interrupt.payload.risk} risk
          </span>
        ) : null}
      </div>
      {props.interrupt.payload?.reason ? (
        <p className="request-reason">{props.interrupt.payload.reason}</p>
      ) : null}
      <div className="approval-decisions" role="group" aria-label="Decision">
        <button
          type="button"
          data-selected={approval.decision === "approve"}
          aria-pressed={approval.decision === "approve"}
          disabled={props.disabled}
          onClick={() => update({ decision: "approve" })}
        >
          Approve
        </button>
        <button
          type="button"
          data-selected={approval.decision === "deny"}
          aria-pressed={approval.decision === "deny"}
          disabled={props.disabled}
          onClick={() => update({ decision: "deny" })}
        >
          Deny
        </button>
      </div>
      {tool ? (
        <details className="approval-arguments">
          <summary>Edit arguments</summary>
          <textarea
            value={approval.argumentsText}
            rows={Math.min(
              12,
              Math.max(4, approval.argumentsText.split("\n").length),
            )}
            spellCheck={false}
            disabled={props.disabled}
            aria-label={`Arguments for ${tool.name}`}
            onChange={(event) =>
              update({ argumentsText: event.currentTarget.value })
            }
          />
        </details>
      ) : null}
      <div className="approval-details">
        <label>
          <span>
            Reason <small>optional</small>
          </span>
          <input
            value={approval.reason}
            disabled={props.disabled}
            placeholder={
              approval.decision === "deny"
                ? "Tell Lyra why this was denied"
                : "Add context for this decision"
            }
            onChange={(event) => update({ reason: event.currentTarget.value })}
          />
        </label>
        {props.interrupt.payload?.rememberable ? (
          <label>
            <span>Remember decision</span>
            <select
              value={approval.remember}
              disabled={props.disabled}
              onChange={(event) =>
                update({
                  remember: event.currentTarget
                    .value as ApprovalDraft["remember"],
                })
              }
            >
              <option value="once">Just this time</option>
              <option value="session">This session</option>
              <option value="project">This project</option>
              <option value="global">Everywhere</option>
            </select>
          </label>
        ) : null}
      </div>
    </section>
  );
}
