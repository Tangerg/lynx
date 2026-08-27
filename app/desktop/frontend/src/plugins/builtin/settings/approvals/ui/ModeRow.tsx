import { Icon, Pressable } from "@/ui";
import {
  agentCommandWasRetired,
  APPROVAL_MODES,
  saveApprovalMode,
  type ApprovalMode,
} from "../application/approvalConfig";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import { useState } from "react";

type ApprovalModeIntent = {
  mode: ApprovalMode;
  settlement: "pending" | "accepted-awaiting-projection";
} | null;

export function ModeRow({ mode }: { mode: ApprovalMode | undefined }) {
  const t = useT();
  const [intent, setIntent] = useState<ApprovalModeIntent>(null);
  const activeIntent =
    intent?.settlement === "accepted-awaiting-projection" && intent.mode === mode ? null : intent;

  const onChange = async (next: ApprovalMode) => {
    if (activeIntent !== null || next === mode) return;
    setIntent({ mode: next, settlement: "pending" });
    try {
      const accepted = await saveApprovalMode(next);
      setIntent((current) =>
        current?.mode === next
          ? { mode: accepted, settlement: "accepted-awaiting-projection" }
          : current,
      );
    } catch (err) {
      setIntent((current) => (current?.mode === next ? null : current));
      if (agentCommandWasRetired(err)) return;
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <div>
      <div className="text-ui-md font-medium text-fg">{t("approvals.mode")}</div>
      <div className="mt-1 text-ui-md leading-body text-fg-muted">{t("approvals.mode.sub")}</div>
      {mode === undefined ? (
        // Until the saved stance loads, show a quiet placeholder rather than
        // selecting a default row — a fake selection could contradict the real
        // mode for a frame.
        <div className="mt-3 h-[184px] rounded-lg bg-sunken" aria-hidden />
      ) : (
        <div className="mt-3 flex flex-col gap-0.5">
          {APPROVAL_MODES.map((o) => {
            const selected = o.value === (activeIntent?.mode ?? mode);
            const saving = o.value === activeIntent?.mode;
            return (
              <Pressable
                key={o.value}
                type="button"
                aria-pressed={selected}
                aria-label={t(o.labelKey)}
                aria-busy={saving || undefined}
                disabled={activeIntent !== null}
                onClick={() => void onChange(o.value)}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-3 text-left transition-colors",
                  selected ? "bg-accent-wash" : "hover:bg-hover",
                )}
              >
                <div className="min-w-0 flex-1">
                  <div
                    className={cn("text-ui-md", selected ? "font-medium text-accent" : "text-fg")}
                  >
                    {t(o.labelKey)}
                  </div>
                  <div className="mt-0.5 text-ui-md leading-body text-fg-muted">{t(o.descKey)}</div>
                </div>
                {saving ? (
                  <Icon name="loop" size="sm" className="shrink-0 animate-spin text-accent" />
                ) : (
                  selected && <Icon name="check" size="md" className="shrink-0 text-accent" />
                )}
              </Pressable>
            );
          })}
        </div>
      )}
    </div>
  );
}
