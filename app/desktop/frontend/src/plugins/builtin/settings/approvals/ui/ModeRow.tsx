import { Icon, Pressable } from "@/ui";
import { APPROVAL_MODES, saveApprovalMode, type ApprovalMode } from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

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
      <div className="text-ui-lg font-medium text-fg">{t("approvals.mode")}</div>
      <div className="mt-1 text-ui-md leading-body text-fg-muted">{t("approvals.mode.sub")}</div>
      {mode === undefined ? (
        // Until the saved stance loads, show a quiet placeholder rather than
        // selecting a default row — a fake selection could contradict the real
        // mode for a frame.
        <div className="mt-3 h-[184px] rounded-lg bg-surface" aria-hidden />
      ) : (
        <div className="mt-3 flex flex-col gap-0.5">
          {APPROVAL_MODES.map((o) => {
            const selected = o.value === mode;
            return (
              <Pressable
                key={o.value}
                type="button"
                aria-pressed={selected}
                aria-label={t(o.labelKey)}
                onClick={() => void onChange(o.value)}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-3 text-left transition-colors",
                  selected ? "bg-accent-wash" : "hover:bg-hover",
                )}
              >
                <div className="min-w-0 flex-1">
                  <div
                    className={cn("text-ui-lg", selected ? "font-medium text-accent" : "text-fg")}
                  >
                    {t(o.labelKey)}
                  </div>
                  <div className="mt-0.5 text-ui-md leading-body text-fg-muted">{t(o.descKey)}</div>
                </div>
                {selected && <Icon name="check" size="md" className="shrink-0 text-accent" />}
              </Pressable>
            );
          })}
        </div>
      )}
    </div>
  );
}
