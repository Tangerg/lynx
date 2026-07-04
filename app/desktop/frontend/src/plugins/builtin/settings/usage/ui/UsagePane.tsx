// The "Usage" settings pane — a cross-session spend dashboard (usage.summary).
// Totals plus per-provider / per-model / per-day breakdowns, summed server-side
// from the durable run history. Read-only; the range selector limits the
// window. Mirrors opencode's `/stats` surface.

import type { ReactNode } from "react";
import { useState } from "react";
import { EmptyState, ProviderIcon, Segmented } from "@/ui";
import { fmtCost, fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import {
  USAGE_RANGES,
  type UsageBreakdownBucket,
  usageTokens,
  useUsageReport,
} from "../application/usageConfig";

// One breakdown section (provider / model / day): a titled list of buckets,
// each a label + its cost + token count, right-aligned and tabular.
function BreakdownSection({
  title,
  buckets,
  icon,
}: {
  title: string;
  buckets: UsageBreakdownBucket[];
  icon?: (key: string) => ReactNode;
}) {
  if (buckets.length === 0) return null;
  return (
    <div className="rounded-[14px] bg-surface p-4">
      <div className="mb-1.5 text-[12px] font-medium text-fg-muted">{title}</div>
      <div className="flex flex-col">
        {buckets.map((b) => (
          <div
            key={b.key}
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-md px-2 py-2 transition-colors hover:bg-fg/[0.04]"
          >
            <div className="flex min-w-0 items-center gap-2">
              {icon?.(b.key)}
              <span className="truncate text-[13px] text-fg">{b.key}</span>
            </div>
            <div className="flex items-center gap-3 font-mono text-[12px] tabular-nums">
              <span className="text-fg-muted">{fmtTokens(usageTokens(b))}</span>
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

export function UsagePane() {
  const t = useT();
  const [sinceDays, setSinceDays] = useState(0);
  const { data, isLoading, isError } = useUsageReport(sinceDays);

  const total = data?.total;
  const totalTokens = total ? usageTokens(total) : 0;
  const hasSpend = totalTokens > 0 || (total?.costUsd ?? 0) > 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="self-end">
        <Segmented
          value={sinceDays}
          options={USAGE_RANGES.map((r) => ({ value: r.days, label: t(r.label) }))}
          onChange={setSinceDays}
          ariaLabel="Usage range"
        />
      </div>

      {isLoading && <div className="text-[12px] text-fg-muted">{t("usage.loading")}</div>}
      {isError && <div className="text-[12px] text-negative">{t("usage.error")}</div>}

      {data && !hasSpend && (
        <EmptyState icon="chart" title={t("usage.empty")} sub={t("usage.empty.sub")} />
      )}

      {data && hasSpend && (
        <>
          <div className="flex flex-col gap-2 rounded-[14px] bg-surface p-4">
            <div className="flex items-baseline justify-between gap-3">
              <span className="text-[12px] font-medium text-fg-muted">{t("usage.total")}</span>
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
