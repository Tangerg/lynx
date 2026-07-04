import { Icon } from "@/ui";
import { APPROVAL_MODES, saveApprovalMode, type ApprovalMode } from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

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
    <div>
      <div className="text-[13px] font-medium text-fg">{t("approvals.mode")}</div>
      <div className="mt-0.5 text-[13px] leading-[1.5] text-fg-muted">
        {t("approvals.mode.sub")}
      </div>
      {mode === undefined ? (
        // Until the saved stance loads, show a quiet placeholder rather than
        // selecting a default row — a fake selection could contradict the real
        // mode for a frame.
        <div className="mt-3 h-[184px] rounded-[14px] bg-surface" aria-hidden />
      ) : (
        <div className="mt-3 flex flex-col gap-0.5">
          {APPROVAL_MODES.map((o) => {
            const selected = o.value === mode;
            return (
              <button
                key={o.value}
                type="button"
                aria-pressed={selected}
                aria-label={t(o.labelKey)}
                onClick={() => void onChange(o.value)}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-3 text-left transition-colors",
                  selected ? "bg-accent/10" : "hover:bg-fg/[0.04]",
                )}
              >
                <div className="min-w-0 flex-1">
                  <div
                    className={cn("text-[14px]", selected ? "font-medium text-accent" : "text-fg")}
                  >
                    {t(o.labelKey)}
                  </div>
                  <div className="mt-0.5 text-[12px] leading-[1.45] text-fg-muted">
                    {t(o.descKey)}
                  </div>
                </div>
                {selected && <Icon name="check" size={15} className="shrink-0 text-accent" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
