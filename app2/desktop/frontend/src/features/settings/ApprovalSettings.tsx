import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import type { ApprovalRule, RuntimeConnection } from "@lyra/runtime-contract";

import {
  forgetApprovalRule,
  getApprovalMode,
  listApprovalRules,
  runtimeQueryKeys,
  setApprovalMode,
} from "../../runtime/runtimeQueries";
import {
  useLocalization,
  type MessageKey,
  type Translate,
} from "../localization/Localization";

type ApprovalMode = "safe" | "balanced" | "yolo";

const modes: Array<{
  id: ApprovalMode;
  name: MessageKey;
  description: MessageKey;
  badge: MessageKey;
}> = [
  {
    id: "safe",
    name: "settings.approval.mode.safe.name",
    description: "settings.approval.mode.safe.description",
    badge: "settings.approval.mode.safe.badge",
  },
  {
    id: "balanced",
    name: "settings.approval.mode.balanced.name",
    description: "settings.approval.mode.balanced.description",
    badge: "settings.approval.mode.balanced.badge",
  },
  {
    id: "yolo",
    name: "settings.approval.mode.yolo.name",
    description: "settings.approval.mode.yolo.description",
    badge: "settings.approval.mode.yolo.badge",
  },
];

interface ApprovalSettingsProps {
  connection: RuntimeConnection;
  sessionId?: string;
}

export function ApprovalSettings(props: ApprovalSettingsProps) {
  const { t, formatNumber } = useLocalization();
  const queryClient = useQueryClient();
  const [confirmingRule, setConfirmingRule] = useState<string>();
  const modeKey = runtimeQueryKeys.approvalMode(props.connection);
  const mode = useQuery({
    queryKey: modeKey,
    queryFn: ({ signal }) => getApprovalMode(props.connection, signal),
    retry: 2,
  });
  const rulesKey = runtimeQueryKeys.approvalRules(
    props.connection,
    props.sessionId ?? "unselected",
  );
  const rules = useQuery({
    queryKey: rulesKey,
    queryFn: ({ signal }) =>
      listApprovalRules(props.connection, props.sessionId ?? "", signal),
    enabled: props.sessionId !== undefined,
    retry: 2,
  });
  const changeMode = useMutation({
    mutationFn: (next: ApprovalMode) => setApprovalMode(props.connection, next),
    onSuccess: (committed) => {
      queryClient.setQueryData(modeKey, committed);
      void queryClient.invalidateQueries({
        queryKey: runtimeQueryKeys.approvals(props.connection),
      });
    },
  });
  const forget = useMutation({
    mutationFn: (id: string) => forgetApprovalRule(props.connection, id),
    onSuccess: () => {
      setConfirmingRule(undefined);
      void queryClient.invalidateQueries({ queryKey: rulesKey });
    },
  });

  return (
    <>
      <section
        className="settings-section"
        aria-labelledby="approval-mode-title"
      >
        <header>
          <div>
            <h2 id="approval-mode-title">
              {t("settings.approval.effectStance")}
            </h2>
            <p>{t("settings.approval.effectStanceDetail")}</p>
          </div>
        </header>
        {mode.isPending ? (
          <ApprovalState>{t("settings.approval.loadingMode")}</ApprovalState>
        ) : mode.isError ? (
          <ApprovalState
            action={t("settings.approval.tryAgain")}
            onAction={() => void mode.refetch()}
          >
            {messageOf(mode.error, t)}
          </ApprovalState>
        ) : (
          <div className="approval-mode-grid">
            {modes.map((candidate) => {
              const selected = mode.data?.mode === candidate.id;
              return (
                <button
                  key={candidate.id}
                  className="approval-mode-card"
                  type="button"
                  data-selected={selected || undefined}
                  aria-pressed={selected}
                  disabled={changeMode.isPending}
                  onClick={() => changeMode.mutate(candidate.id)}
                >
                  <span>
                    <strong>{t(candidate.name)}</strong>
                    <small>{t(candidate.badge)}</small>
                  </span>
                  <p>{t(candidate.description)}</p>
                </button>
              );
            })}
          </div>
        )}
        {changeMode.isError ? (
          <p className="settings-inline-error" role="alert">
            {messageOf(changeMode.error, t)}
          </p>
        ) : null}
      </section>

      <section
        className="settings-section"
        aria-labelledby="approval-rules-title"
      >
        <header>
          <div>
            <h2 id="approval-rules-title">
              {t("settings.approval.remembered")}
            </h2>
            <p>{t("settings.approval.rememberedDetail")}</p>
          </div>
          {rules.data ? (
            <span className="approval-rule-count">
              {t("settings.approval.visibleCount", {
                count: formatNumber(rules.data.rules.length),
              })}
            </span>
          ) : null}
        </header>
        {props.sessionId === undefined ? (
          <ApprovalState>{t("settings.approval.selectSession")}</ApprovalState>
        ) : rules.isPending ? (
          <ApprovalState>{t("settings.approval.loadingRules")}</ApprovalState>
        ) : rules.isError ? (
          <ApprovalState
            action={t("settings.approval.tryAgain")}
            onAction={() => void rules.refetch()}
          >
            {messageOf(rules.error, t)}
          </ApprovalState>
        ) : rules.data.rules.length === 0 ? (
          <ApprovalState>{t("settings.approval.empty")}</ApprovalState>
        ) : (
          <div className="approval-rule-list">
            {rules.data.rules.map((rule) => (
              <ApprovalRuleCard
                key={rule.id}
                rule={rule}
                confirming={confirmingRule === rule.id}
                pending={forget.isPending && forget.variables === rule.id}
                onConfirm={() => setConfirmingRule(rule.id)}
                onCancel={() => setConfirmingRule(undefined)}
                onForget={() => forget.mutate(rule.id)}
              />
            ))}
          </div>
        )}
        {forget.isError ? (
          <p className="settings-inline-error" role="alert">
            {messageOf(forget.error, t)}
          </p>
        ) : null}
      </section>
    </>
  );
}

function ApprovalRuleCard(props: {
  rule: ApprovalRule;
  confirming: boolean;
  pending: boolean;
  onConfirm(): void;
  onCancel(): void;
  onForget(): void;
}) {
  const { t } = useLocalization();
  const subject =
    props.rule.subject === ""
      ? t("settings.approval.everyInvocation")
      : props.rule.subject;
  return (
    <article className="approval-rule-card" data-decision={props.rule.decision}>
      <header>
        <div>
          <span className="approval-rule-verdict">
            {decisionName(props.rule.decision, t)}
          </span>
          <span className="approval-rule-scope">
            {scopeName(props.rule.scope, t)}
          </span>
        </div>
        {props.confirming ? (
          <div className="approval-forget-confirm">
            <span>{t("settings.approval.forgetQuestion")}</span>
            <button
              type="button"
              disabled={props.pending}
              onClick={props.onCancel}
            >
              {t("settings.approval.cancel")}
            </button>
            <button
              className="danger"
              type="button"
              disabled={props.pending}
              onClick={props.onForget}
            >
              {props.pending
                ? t("settings.approval.forgetting")
                : t("settings.approval.forget")}
            </button>
          </div>
        ) : (
          <button
            className="text-action danger"
            type="button"
            onClick={props.onConfirm}
          >
            {t("settings.approval.forget")}
          </button>
        )}
      </header>
      <div className="approval-rule-key">
        <strong>{props.rule.tool}</strong>
        <code title={subject}>{subject}</code>
      </div>
      {props.rule.dir ? (
        <p title={props.rule.dir}>
          {t("settings.approval.projectPath", { path: props.rule.dir })}
        </p>
      ) : null}
    </article>
  );
}

function ApprovalState(props: {
  children: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <div className="settings-state">
      <p>{props.children}</p>
      {props.action && props.onAction ? (
        <button
          className="secondary-action"
          type="button"
          onClick={props.onAction}
        >
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function scopeName(scope: string, t: Translate) {
  switch (scope) {
    case "session":
      return t("settings.approval.scope.session");
    case "project":
      return t("settings.approval.scope.project");
    case "global":
      return t("settings.approval.scope.global");
    default:
      return scope;
  }
}

function decisionName(decision: string, t: Translate) {
  switch (decision) {
    case "approve":
      return t("settings.approval.decision.approve");
    case "deny":
      return t("settings.approval.decision.deny");
    default:
      return decision;
  }
}

function messageOf(error: unknown, t: Translate) {
  return error instanceof Error
    ? error.message
    : t("settings.approval.requestFailed");
}
