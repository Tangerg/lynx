// Built-in plugin: "Approvals" settings pane (B9). Sets the runtime's global
// approval stance (approval.getMode / setMode) and manages the persistent
// fine-grained approval rules (approval.listRules / forgetRule).
//
// Approval is a core capability (not feature-gated per the backend), but the
// approval.* methods only exist on a B9 runtime — a pre-B9 one rejects getMode,
// so the whole pane degrades to an inert "unavailable" state.

import type { ApprovalModeValue, ApprovalRuleInfo } from "@/lib/data/queries";
import { DataView, EmptyState, Icon, Segmented } from "@/components/common";
import {
  APPROVAL_MODES,
  forgetRule,
  setApprovalMode,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { isUnsupportedMethod, rpcErrorText } from "@/lib/agent/errorCopy";
import { useActiveSession } from "@/lib/agent/useActiveSession";
import { useApprovalMode, useApprovalRules } from "@/lib/data/queries";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { SettingRow } from "../SettingRow";

const SCOPE_CHIP: Record<ApprovalRuleInfo["scope"], string> = {
  session: "border-line bg-surface-2 text-fg-muted",
  project: "border-accent/30 bg-accent/10 text-accent",
  global: "border-warning/30 bg-warning/12 text-warning",
};

function ModeRow({ mode }: { mode: ApprovalModeValue | undefined }) {
  const t = useT();
  const onChange = async (next: ApprovalModeValue) => {
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <SettingRow label={t("approvals.mode")} sub={t("approvals.mode.sub")}>
      {mode === undefined ? (
        // Until the saved stance loads, show a quiet placeholder rather than
        // defaulting the control to "balanced" — a fake selection that could
        // contradict the real mode for a frame.
        <div className="h-7 w-[260px] rounded-md bg-surface-2 opacity-50" aria-hidden />
      ) : (
        <Segmented
          value={mode}
          options={APPROVAL_MODES.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
          onChange={(v) => void onChange(v)}
          ariaLabel={t("approvals.mode.aria")}
        />
      )}
    </SettingRow>
  );
}

function RulesRow() {
  const t = useT();
  const sessionId = useActiveSession()?.id;
  const { data, isLoading, isError, error } = useApprovalRules(
    sessionId ? { sessionId } : undefined,
  );
  const forget = async (id: string) => {
    try {
      await forgetRule(id);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.forget"));
    }
  };
  // Clear-all loops the visible ids — the wire forgets one rule at a time.
  const forgetAll = async (rows: ApprovalRuleInfo[]) => {
    for (const r of rows) await forget(r.id);
  };

  return (
    <SettingRow label={t("approvals.rules")} sub={t("approvals.rules.sub")} align="start">
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        error={
          isUnsupportedMethod(error)
            ? {
                icon: "shield",
                title: t("runtime.unsupported.title"),
                sub: t("runtime.unsupported.sub"),
              }
            : undefined
        }
        empty={{
          icon: "check",
          title: t("approvals.rules.empty"),
          sub: t("approvals.rules.emptySub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col gap-1.5">
            <div className="flex justify-end">
              <button
                type="button"
                className="text-[12px] text-fg-muted hover:text-fg"
                onClick={() => void forgetAll(rows)}
              >
                {t("approvals.clearAll")}
              </button>
            </div>
            {rows.map((r) => (
              <div
                key={r.id}
                className="flex items-center gap-2 rounded-md bg-surface-2 px-2.5 py-1.5 light:bg-surface-3"
              >
                <span
                  className={cn(
                    "shrink-0 rounded-xs border px-1.5 py-px font-mono text-[10px] font-semibold",
                    SCOPE_CHIP[r.scope],
                  )}
                >
                  {t(`approvals.scope.${r.scope}`)}
                </span>
                <span
                  className={cn(
                    "shrink-0 text-[11px] font-semibold",
                    r.decision === "deny" ? "text-negative" : "text-success",
                  )}
                >
                  {r.decision === "deny" ? t("approvals.deny") : t("approvals.allow")}
                </span>
                <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-fg">
                  {r.tool}
                  {r.subject ? <span className="text-fg-muted"> · {r.subject}</span> : null}
                  {r.dir ? <span className="text-fg-faint"> — {r.dir}</span> : null}
                </span>
                <button
                  type="button"
                  aria-label={t("approvals.forget", { tool: r.tool })}
                  className="shrink-0 text-fg-faint hover:text-fg"
                  onClick={() => void forget(r.id)}
                >
                  <Icon name="x" size={13} />
                </button>
              </div>
            ))}
          </div>
        )}
      </DataView>
    </SettingRow>
  );
}

function ApprovalsPane() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError) {
    return (
      <EmptyState
        icon="shield"
        title={t("approvals.unavailable")}
        sub={t("approvals.unavailable.sub")}
      />
    );
  }
  return (
    <div>
      <ModeRow mode={mode} />
      <RulesRow />
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.approvals-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "approvals",
      label: "settings.pane.approvals",
      group: "agent",
      icon: "shield",
      order: 55,
      component: ApprovalsPane,
    });
  },
});
