// Built-in plugin: "Approvals" settings pane (B9, 613). Sets the runtime's
// global approval stance (approval.getMode / setMode) and manages the
// per-session "remembered" tool decisions (listRemembered / forget).
//
// Approval is a core capability (not feature-gated per the backend), but the
// approval.* methods only exist on a B9 runtime — a pre-B9 one rejects getMode,
// so the whole pane degrades to an inert "unavailable" state.

import type { ApprovalModeValue } from "@/lib/data/queries";
import { DataView, EmptyState, Icon, Segmented } from "@/components/common";
import { forgetDecision, setApprovalMode } from "@/lib/agent/approvalConfig";
import { rpcErrorText } from "@/lib/agent/errorCopy";
import { useActiveSession } from "@/lib/agent/useActiveSession";
import { useApprovalMode, useRememberedDecisions } from "@/lib/data/queries";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { SettingRow } from "../SettingRow";

// `label` is an i18n key resolved at render (ModeRow's t()); module scope can't
// call the hook, so the mapping happens inside the component.
const MODE_OPTIONS: { value: ApprovalModeValue; label: string }[] = [
  { value: "readOnly", label: "approvals.mode.readOnly" },
  { value: "safe", label: "approvals.mode.safe" },
  { value: "balanced", label: "approvals.mode.balanced" },
  { value: "yolo", label: "approvals.mode.auto" },
];

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
      <Segmented
        value={mode ?? "balanced"}
        options={MODE_OPTIONS.map((o) => ({ value: o.value, label: t(o.label) }))}
        onChange={(v) => void onChange(v)}
        ariaLabel={t("approvals.mode.aria")}
      />
    </SettingRow>
  );
}

function RememberedRow() {
  const t = useT();
  const sessionId = useActiveSession()?.id;
  const { data, isLoading, isError } = useRememberedDecisions(
    sessionId ? { sessionId } : undefined,
  );
  const forget = async (tool?: string) => {
    if (!sessionId) return;
    try {
      await forgetDecision(sessionId, tool);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.forget"));
    }
  };

  return (
    <SettingRow label={t("approvals.remembered")} sub={t("approvals.remembered.sub")} align="start">
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        empty={{
          icon: "check",
          title: t("approvals.remembered.empty"),
          sub: t("approvals.remembered.emptySub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col gap-1.5">
            <div className="flex justify-end">
              <button
                type="button"
                className="text-[12px] text-fg-muted hover:text-fg"
                onClick={() => void forget()}
              >
                {t("approvals.clearAll")}
              </button>
            </div>
            {rows.map((d) => (
              <div
                key={d.tool}
                className="flex items-center justify-between rounded-md bg-surface-2 px-2.5 py-1.5 light:bg-surface-3"
              >
                <span className="flex items-center gap-2 font-mono text-[12px] text-fg">
                  <span className={d.decision === "approve" ? "text-accent" : "text-negative"}>
                    {d.decision === "approve" ? t("approvals.allow") : t("approvals.deny")}
                  </span>
                  {d.tool}
                </span>
                <button
                  type="button"
                  aria-label={t("approvals.forget", { tool: d.tool })}
                  className="text-fg-faint hover:text-fg"
                  onClick={() => void forget(d.tool)}
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
      <RememberedRow />
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
      icon: "shield",
      order: 55,
      component: ApprovalsPane,
    });
  },
});
