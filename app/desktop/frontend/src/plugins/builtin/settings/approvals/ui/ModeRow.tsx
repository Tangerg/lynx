import { Segmented } from "@/components/common";
import { APPROVAL_MODES, saveApprovalMode, type ApprovalMode } from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { SettingRow } from "../../SettingRow";

export function ModeRow({ mode }: { mode: ApprovalMode | undefined }) {
  const t = useT();
  const onChange = async (next: ApprovalMode) => {
    try {
      await saveApprovalMode(next);
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
