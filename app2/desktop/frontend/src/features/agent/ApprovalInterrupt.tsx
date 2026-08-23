import type { Interrupt } from "@lyra/runtime-contract";

import { useLocalization } from "../localization/Localization";
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
  const { t } = useLocalization();
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
          <small>{t("approval.title")}</small>
          <h4>{tool?.name ?? t("approval.toolExecution")}</h4>
        </div>
        {props.interrupt.payload?.risk ? (
          <span
            className="risk-badge"
            data-risk={props.interrupt.payload.risk}
          >
            {t("approval.risk", { risk: props.interrupt.payload.risk })}
          </span>
        ) : null}
      </div>
      {props.interrupt.payload?.reason ? (
        <p className="request-reason">{props.interrupt.payload.reason}</p>
      ) : null}
      <div
        className="approval-decisions"
        role="group"
        aria-label={t("approval.decision")}
      >
        <button
          type="button"
          data-selected={approval.decision === "approve"}
          aria-pressed={approval.decision === "approve"}
          disabled={props.disabled}
          onClick={() => update({ decision: "approve" })}
        >
          {t("approval.approve")}
        </button>
        <button
          type="button"
          data-selected={approval.decision === "deny"}
          aria-pressed={approval.decision === "deny"}
          disabled={props.disabled}
          onClick={() => update({ decision: "deny" })}
        >
          {t("approval.deny")}
        </button>
      </div>
      {tool ? (
        <details className="approval-arguments">
          <summary>{t("approval.editArguments")}</summary>
          <textarea
            dir="ltr"
            value={approval.argumentsText}
            rows={Math.min(
              12,
              Math.max(4, approval.argumentsText.split("\n").length),
            )}
            spellCheck={false}
            disabled={props.disabled}
            aria-label={t("approval.argumentsFor", { tool: tool.name })}
            onChange={(event) =>
              update({ argumentsText: event.currentTarget.value })
            }
          />
        </details>
      ) : null}
      <div className="approval-details">
        <label>
          <span>
            {t("approval.reason")} <small>{t("approval.optional")}</small>
          </span>
          <input
            value={approval.reason}
            disabled={props.disabled}
            placeholder={
              approval.decision === "deny"
                ? t("approval.denialReasonPlaceholder")
                : t("approval.contextPlaceholder")
            }
            onChange={(event) => update({ reason: event.currentTarget.value })}
          />
        </label>
        {props.interrupt.payload?.rememberable ? (
          <label>
            <span>{t("approval.rememberDecision")}</span>
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
              <option value="once">{t("approval.rememberOnce")}</option>
              <option value="session">{t("approval.rememberSession")}</option>
              <option value="project">{t("approval.rememberProject")}</option>
              <option value="global">{t("approval.rememberGlobal")}</option>
            </select>
          </label>
        ) : null}
      </div>
    </section>
  );
}
