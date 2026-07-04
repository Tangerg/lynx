import { DataView, Icon } from "@/ui";
import {
  forgetApprovalRule,
  forgetApprovalRules,
  type ApprovalRuleConfig,
  useApprovalRuleConfigs,
} from "../application/approvalConfig";
import { isUnsupportedMethod, rpcErrorText } from "@/lib/rpcErrors";
import { useActiveSession } from "@/plugins/builtin/agent/public/session";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { SettingRow } from "../../SettingRow";

const SCOPE_CHIP: Record<ApprovalRuleConfig["scope"], string> = {
  session: "border-transparent bg-surface-2 text-fg-muted",
  project: "border-accent/30 bg-accent/10 text-accent",
  global: "border-warning/30 bg-warning/12 text-warning",
};

export function RulesRow() {
  const t = useT();
  const sessionId = useActiveSession()?.id;
  const { data, isLoading, isError, error } = useApprovalRuleConfigs(sessionId);
  const forget = async (id: string) => {
    try {
      await forgetApprovalRule(id);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.forget"));
    }
  };
  const forgetAll = async (rows: ApprovalRuleConfig[]) => {
    try {
      await forgetApprovalRules(rows);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.forget"));
    }
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
                    "shrink-0 rounded-xs border-[0.5px] px-1.5 py-px font-mono text-[10px] font-semibold",
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
