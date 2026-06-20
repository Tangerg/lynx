// Built-in plugin: "Usage" settings pane — a cross-session spend dashboard
// (usage.summary). Totals plus per-provider / per-model / per-day breakdowns,
// summed server-side from the durable run history. Read-only; the range selector
// limits the window. Mirrors opencode's `/stats` surface.

import type { UsageBucket } from "@/rpc";
import type { ReactNode } from "react";
import { useState } from "react";
import { Icon, ProviderIcon } from "@/components/common";
import { fmtCost, fmtTokens } from "@/lib/format";
import { useUsageSummary } from "@/lib/data/useUsage";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

const RANGES = [
  { days: 0, label: "usage.range.all" },
  { days: 30, label: "usage.range.30d" },
  { days: 7, label: "usage.range.7d" },
] as const;

function tokensOf(b: { inputTokens?: number; outputTokens?: number }): number {
  return (b.inputTokens ?? 0) + (b.outputTokens ?? 0);
}

// One breakdown section (provider / model / day): a titled list of buckets,
// each a label + its cost + token count, right-aligned and tabular.
function BreakdownSection({
  title,
  buckets,
  icon,
}: {
  title: string;
  buckets: UsageBucket[];
  icon?: (key: string) => ReactNode;
}) {
  if (buckets.length === 0) return null;
  return (
    <div className="flex flex-col gap-1.5">
      <div className="text-[11px] font-semibold uppercase tracking-wide text-fg-faint">{title}</div>
      <div className="flex flex-col gap-px overflow-hidden rounded-lg">
        {buckets.map((b) => (
          <div
            key={b.key}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 bg-canvas px-3 py-2"
          >
            <div className="flex min-w-0 items-center gap-2">
              {icon?.(b.key)}
              <span className="truncate text-[13px] text-fg">{b.key}</span>
            </div>
            <div className="flex items-center gap-3 font-mono text-[12px] tabular-nums">
              <span className="text-fg-muted">{fmtTokens(tokensOf(b))}</span>
              {b.costUsd !== undefined && (
                <span className="w-16 text-right text-fg">{fmtCost(b.costUsd)}</span>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function UsagePane() {
  const t = useT();
  const [sinceDays, setSinceDays] = useState(0);
  const { data, isLoading, isError } = useUsageSummary(sinceDays);

  const total = data?.total;
  const totalTokens = total ? tokensOf(total) : 0;
  const hasSpend = totalTokens > 0 || (total?.costUsd ?? 0) > 0;

  return (
    <div className="flex flex-col gap-4">
      {/* Range selector */}
      <div className="flex items-center gap-1 self-end rounded-full bg-surface-2 p-0.5">
        {RANGES.map((r) => (
          <button
            key={r.days}
            type="button"
            onClick={() => setSinceDays(r.days)}
            className={cn(
              "h-6 rounded-full px-2.5 text-[11.5px] font-medium transition-colors",
              sinceDays === r.days ? "bg-surface text-fg shadow-sm" : "text-fg-faint hover:text-fg",
            )}
          >
            {t(r.label)}
          </button>
        ))}
      </div>

      {isLoading && <div className="text-[12px] text-fg-faint">{t("usage.loading")}</div>}
      {isError && <div className="text-[12px] text-negative">{t("usage.error")}</div>}

      {data && !hasSpend && (
        <div className="flex flex-col items-center gap-2 rounded-lg bg-surface-2 px-4 py-10 text-center">
          <Icon name="chart" size={22} className="text-fg-faint" />
          <div className="text-[13px] font-medium text-fg">{t("usage.empty")}</div>
          <div className="text-[11.5px] text-fg-faint">{t("usage.empty.sub")}</div>
        </div>
      )}

      {data && hasSpend && (
        <>
          {/* Total card */}
          <div className="flex flex-col gap-2 rounded-lg bg-surface-2 p-4">
            <div className="flex items-baseline justify-between gap-3">
              <span className="text-[12px] font-semibold uppercase tracking-wide text-fg-faint">
                {t("usage.total")}
              </span>
              <span className="font-mono text-[22px] font-semibold tabular-nums text-fg">
                {total?.costUsd !== undefined ? fmtCost(total.costUsd) : "—"}
              </span>
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-[12px] tabular-nums text-fg-muted">
              <span>↑{fmtTokens(total?.inputTokens ?? 0)}</span>
              <span>↓{fmtTokens(total?.outputTokens ?? 0)}</span>
              {(total?.cacheReadTokens ?? 0) > 0 && (
                <span className="text-fg-faint">
                  {t("usage.cache")} {fmtTokens(total?.cacheReadTokens ?? 0)}
                </span>
              )}
              <span className="text-fg-faint">
                · {t("usage.sessions", { count: data.sessions ?? 0 })} ·{" "}
                {t("usage.runs", { count: data.runs ?? 0 })}
              </span>
            </div>
          </div>

          <BreakdownSection
            title={t("usage.byProvider")}
            buckets={data.byProvider ?? []}
            icon={(key) => <ProviderIcon provider={key} size={16} />}
          />
          <BreakdownSection title={t("usage.byModel")} buckets={data.byModel ?? []} />
          <BreakdownSection title={t("usage.byDay")} buckets={data.byDay ?? []} />
        </>
      )}
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.usage-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "usage",
      label: "settings.pane.usage",
      icon: "chart",
      order: 55, // just after Providers (50)
      component: UsagePane,
    });
  },
});
