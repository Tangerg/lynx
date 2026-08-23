import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import type {
  ModelUsage,
  RuntimeConnection,
  UsageBucket,
} from "@lyra/runtime-contract";

import {
  loadSessionUsage,
  loadUsageSummary,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import {
  useLocalization,
  type MessageKey,
  type Translate,
} from "../localization/Localization";

type NumberFormatter = (
  value: number,
  options?: Intl.NumberFormatOptions,
) => string;

const usagePeriods = [
  { label: "settings.usage.sevenDays", value: 7 },
  { label: "settings.usage.thirtyDays", value: 30 },
  { label: "settings.usage.allTime", value: 0 },
] as const satisfies ReadonlyArray<{ label: MessageKey; value: number }>;

interface UsageSettingsProps {
  connection: RuntimeConnection;
  sessionId?: string;
}

export function UsageSettings(props: UsageSettingsProps) {
  const { t, formatNumber } = useLocalization();
  const [sinceDays, setSinceDays] = useState(30);
  const summary = useQuery({
    queryKey: runtimeQueryKeys.usageSummary(props.connection, sinceDays),
    queryFn: ({ signal }) =>
      loadUsageSummary(props.connection, { sinceDays }, signal),
    retry: 2,
  });
  const session = useQuery({
    queryKey: runtimeQueryKeys.sessionUsage(
      props.connection,
      props.sessionId ?? "unselected",
    ),
    queryFn: ({ signal }) =>
      loadSessionUsage(props.connection, props.sessionId ?? "", signal),
    enabled: props.sessionId !== undefined,
    retry: 2,
  });

  return (
    <div className="usage-settings">
      <section className="settings-section">
        <header>
          <div>
            <h2>{t("settings.usage.runtime")}</h2>
            <p>{t("settings.usage.runtimeDetail")}</p>
          </div>
          <div className="usage-period" aria-label={t("settings.usage.period")}>
            {usagePeriods.map((period) => (
              <button
                key={period.value}
                type="button"
                aria-pressed={sinceDays === period.value}
                onClick={() => setSinceDays(period.value)}
              >
                {t(period.label)}
              </button>
            ))}
          </div>
        </header>
        {summary.isPending ? (
          <UsageState label={t("settings.usage.loading")} />
        ) : summary.isError ? (
          <UsageState
            label={messageOf(summary.error, t)}
            action={t("settings.usage.retry")}
            onAction={() => void summary.refetch()}
          />
        ) : summary.data ? (
          <>
            <div className="usage-metrics">
              <UsageMetric
                label={t("settings.usage.tokens")}
                value={formatTokens(
                  totalTokens(summary.data.total),
                  formatNumber,
                )}
              />
              <UsageMetric
                label={t("settings.usage.cost")}
                value={formatCost(summary.data.total.costUsd, t, formatNumber)}
              />
              <UsageMetric
                label={t("settings.usage.runs")}
                value={formatNumber(summary.data.runs ?? 0)}
              />
              <UsageMetric
                label={t("settings.usage.sessions")}
                value={formatNumber(summary.data.sessions ?? 0)}
              />
            </div>
            <p className="usage-cost-note">{t("settings.usage.costNote")}</p>
            <UsageBreakdown
              title={t("settings.usage.providers")}
              values={summary.data.byProvider ?? []}
              t={t}
              formatNumber={formatNumber}
            />
            <UsageBreakdown
              title={t("settings.usage.models")}
              values={summary.data.byModel ?? []}
              t={t}
              formatNumber={formatNumber}
            />
            <UsageBreakdown
              title={t("settings.usage.days")}
              values={summary.data.byDay ?? []}
              t={t}
              formatNumber={formatNumber}
            />
          </>
        ) : null}
      </section>

      <section className="settings-section">
        <header>
          <div>
            <h2>{t("settings.usage.selectedSession")}</h2>
            <p>{t("settings.usage.selectedSessionDetail")}</p>
          </div>
        </header>
        {props.sessionId === undefined ? (
          <UsageState label={t("settings.usage.selectSession")} />
        ) : session.isPending ? (
          <UsageState label={t("settings.usage.loadingSession")} />
        ) : session.isError ? (
          <UsageState
            label={messageOf(session.error, t)}
            action={t("settings.usage.retry")}
            onAction={() => void session.refetch()}
          />
        ) : session.data ? (
          <div className="usage-metrics usage-session-metrics">
            <UsageMetric
              label={t("settings.usage.tokens")}
              value={formatTokens(totalTokens(session.data), formatNumber)}
            />
            <UsageMetric
              label={t("settings.usage.cost")}
              value={formatCost(session.data.costUsd, t, formatNumber)}
            />
            <UsageMetric
              label={t("settings.usage.models")}
              value={formatNumber(
                Object.keys(session.data.byModel ?? {}).length,
              )}
            />
          </div>
        ) : null}
      </section>
    </div>
  );
}

function UsageMetric(props: { label: string; value: string }) {
  return (
    <div>
      <span>{props.label}</span>
      <strong>{props.value}</strong>
    </div>
  );
}

function UsageBreakdown(props: {
  title: string;
  values: UsageBucket[];
  t: Translate;
  formatNumber: NumberFormatter;
}) {
  if (props.values.length === 0) return null;
  return (
    <section className="usage-breakdown" aria-label={props.title}>
      <h3>{props.title}</h3>
      <div>
        {props.values.slice(0, 10).map((value) => (
          <article key={value.key}>
            <strong title={value.key}>{value.key}</strong>
            <span>{formatTokens(totalTokens(value), props.formatNumber)}</span>
            <span>
              {formatCost(value.costUsd, props.t, props.formatNumber)}
            </span>
            <small>
              {props.t(
                (value.runs ?? 0) === 1
                  ? "settings.usage.runCountOne"
                  : "settings.usage.runCountMany",
                { count: props.formatNumber(value.runs ?? 0) },
              )}
            </small>
          </article>
        ))}
      </div>
    </section>
  );
}

function UsageState(props: {
  label: string;
  action?: string;
  onAction?: () => void;
}) {
  return (
    <div className="settings-state">
      <p>{props.label}</p>
      {props.action && props.onAction ? (
        <button type="button" onClick={props.onAction}>
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function totalTokens(usage: ModelUsage): number {
  return (usage.inputTokens ?? 0) + (usage.outputTokens ?? 0);
}

function formatTokens(value: number, formatNumber: NumberFormatter): string {
  return formatNumber(value, {
    notation: value >= 1_000 ? "compact" : "standard",
    maximumFractionDigits: value >= 1_000_000 ? 2 : value >= 1_000 ? 1 : 0,
  });
}

function formatCost(
  value: number | undefined,
  t: Translate,
  formatNumber: NumberFormatter,
): string {
  return value === undefined
    ? t("settings.usage.unknown")
    : formatNumber(value, {
        style: "currency",
        currency: "USD",
        minimumFractionDigits: 4,
        maximumFractionDigits: 4,
      });
}

function messageOf(error: unknown, t: Translate): string {
  return error instanceof Error
    ? error.message
    : t("settings.usage.loadFailed");
}
