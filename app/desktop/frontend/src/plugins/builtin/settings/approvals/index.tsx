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
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { SettingRow } from "../SettingRow";

const MODE_OPTIONS: { value: ApprovalModeValue; label: string }[] = [
  { value: "readOnly", label: "Read-only" },
  { value: "safe", label: "Safe" },
  { value: "balanced", label: "Balanced" },
  { value: "yolo", label: "Auto" },
];

function ModeRow({ mode }: { mode: ApprovalModeValue | undefined }) {
  const onChange = async (next: ApprovalModeValue) => {
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? "Couldn't change the approval mode.");
    }
  };
  return (
    <SettingRow label="Mode" sub="How much the agent may do without asking each time">
      <Segmented
        value={mode ?? "balanced"}
        options={MODE_OPTIONS}
        onChange={(v) => void onChange(v)}
        ariaLabel="Approval mode"
      />
    </SettingRow>
  );
}

function RememberedRow() {
  const sessionId = useActiveSession()?.id;
  const { data, isLoading, isError } = useRememberedDecisions(
    sessionId ? { sessionId } : undefined,
  );
  const forget = async (tool?: string) => {
    if (!sessionId) return;
    try {
      await forgetDecision(sessionId, tool);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? "Couldn't clear the decision.");
    }
  };

  return (
    <SettingRow label="Remembered" sub="Tools you set to skip approval this session" align="start">
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        empty={{
          icon: "check",
          title: "Nothing remembered",
          sub: "Approve or deny with remember to add one.",
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
                Clear all
              </button>
            </div>
            {rows.map((d) => (
              <div
                key={d.tool}
                className="flex items-center justify-between rounded-md bg-surface-2 px-2.5 py-1.5 light:bg-surface-3"
              >
                <span className="flex items-center gap-2 font-mono text-[12px] text-fg">
                  <span className={d.decision === "approve" ? "text-accent" : "text-negative"}>
                    {d.decision === "approve" ? "allow" : "deny"}
                  </span>
                  {d.tool}
                </span>
                <button
                  type="button"
                  aria-label={`Forget ${d.tool}`}
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
  const { data: mode, isError } = useApprovalMode();
  if (isError) {
    return (
      <EmptyState
        icon="shield"
        title="Approval controls unavailable"
        sub="This runtime doesn't expose approval-mode control."
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
      label: "Approvals",
      icon: "shield",
      order: 55,
      component: ApprovalsPane,
    });
  },
});
