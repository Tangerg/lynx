import { fmtCost, fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { useActiveSessionId, useAgentSessionUsage } from "@/plugins/builtin/agent/public/session";
import { sessionUsageReadout } from "../application/sessionUsageReadout";

export function SessionUsageChip() {
  const t = useT();
  const sessionId = useActiveSessionId();
  const { data } = useAgentSessionUsage(sessionId || undefined);
  const readout = sessionUsageReadout(data);
  if (!readout) return null;

  return (
    <span
      title={t("usage.session.hint")}
      className="hidden shrink-0 items-center gap-1.5 whitespace-nowrap font-mono text-ui-xs tracking-tight text-fg-faint tabular-nums sm:inline-flex"
    >
      <span>↑{fmtTokens(readout.inputTokens)}</span>
      <span>↓{fmtTokens(readout.outputTokens)}</span>
      {readout.costUsd !== undefined && <span>·&nbsp;{fmtCost(readout.costUsd)}</span>}
    </span>
  );
}
