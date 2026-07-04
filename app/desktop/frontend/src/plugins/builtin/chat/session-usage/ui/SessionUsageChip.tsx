import { fmtCost, fmtTokens } from "@/lib/format";
import { useSessionUsage } from "@/lib/data/useUsage";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { sessionUsageReadout } from "../application/sessionUsageReadout";

export function SessionUsageChip() {
  const t = useT();
  const sessionId = useActiveSessionId();
  const { data } = useSessionUsage(sessionId || undefined);
  const readout = sessionUsageReadout(data);
  if (!readout) return null;

  return (
    <div className="flex justify-end pt-1">
      <span
        title={t("usage.session.hint")}
        className="inline-flex h-5 items-center gap-1.5 rounded-sm font-mono text-[11px] text-fg-muted tracking-tight whitespace-nowrap tabular-nums"
      >
        <span className="text-fg-soft">{t("usage.session.label")}</span>
        <span>↑{fmtTokens(readout.inputTokens)}</span>
        <span>↓{fmtTokens(readout.outputTokens)}</span>
        {readout.costUsd !== undefined && <span>·&nbsp;{fmtCost(readout.costUsd)}</span>}
      </span>
    </div>
  );
}
