// Built-in plugin: the session-cumulative usage chip in the chat header
// (chat.banner.top). A subtle, right-aligned readout of the active session's
// total token spend + cost, summed server-side over its finished runs
// (usage.session) — distinct from the composer's live current-run chip. Hidden
// until the session has recorded spend, so a fresh session shows nothing.

import { fmtCost, fmtTokens } from "@/lib/format";
import { useSessionUsage } from "@/lib/data/useUsage";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { definePlugin } from "@/plugins/sdk";

function SessionUsageChip() {
  const t = useT();
  const sessionId = useActiveSessionId();
  const { data } = useSessionUsage(sessionId || undefined);
  if (!data) return null;
  const input = data.inputTokens ?? 0;
  const output = data.outputTokens ?? 0;
  if (input + output === 0 && (data.costUsd ?? 0) === 0) return null;
  return (
    <div className="flex justify-end pt-1">
      <span
        title={t("usage.session.hint")}
        className="inline-flex h-5 items-center gap-1.5 rounded-sm font-mono text-[11px] text-fg-muted tracking-tight whitespace-nowrap tabular-nums"
      >
        <span className="text-fg-soft">{t("usage.session.label")}</span>
        <span>↑{fmtTokens(input)}</span>
        <span>↓{fmtTokens(output)}</span>
        {data.costUsd !== undefined && <span>·&nbsp;{fmtCost(data.costUsd)}</span>}
      </span>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.session-usage",
  version: "1.0.0",
  setup({ host }) {
    // order 10 → below plan-progress (order 0) when both show.
    host.layout.register("chat.banner.top", {
      id: "session-usage",
      order: 10,
      component: SessionUsageChip,
    });
  },
});
