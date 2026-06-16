// Diagnostics view — renders the local telemetry sink (lib/observability):
// the OTel triad as three tabs. Dev-time triage, not a dashboard.
//
// Perf: telemetry volume is high, so spans/logs are bounded ring buffers in
// the store and rendered with @tanstack/react-virtual (only on-screen rows
// mount). Each panel subscribes to ONLY its signal's slice, so switching
// tabs / a metrics flush never re-renders the trace or log list. The Traces
// tab + its span-detail rows live in ./TracesPanel; shared list chrome (Row /
// Cell / Empty / VirtualList) in ./primitives.

import type { MetricRow } from "@/lib/observability/stores";
import { useTelemetryStore } from "@/lib/observability/stores";
import { useMemo, useState } from "react";
import { Segmented } from "@/components/common";
import { Cell, Empty, Row, VirtualList } from "./primitives";
import { TracesPanel } from "./TracesPanel";

type Signal = "traces" | "metrics" | "logs";

const SIGNALS = [
  { value: "traces" as const, label: "Traces" },
  { value: "metrics" as const, label: "Metrics" },
  { value: "logs" as const, label: "Logs" },
];

export function DiagnosticsView() {
  const [signal, setSignal] = useState<Signal>("traces");
  const clear = useTelemetryStore((s) => s.clear);

  return (
    <div className="flex h-full flex-col gap-3 p-6">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-[15px] font-semibold text-fg">Diagnostics</div>
          <div className="mt-0.5 text-[12px] text-fg-muted">
            Live OpenTelemetry — traces / metrics / logs. In-memory only (bounded); the durable
            record leaves via OTLP. "Clear" resets the buffers.
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Segmented
            value={signal}
            options={SIGNALS}
            onChange={setSignal}
            ariaLabel="Telemetry signal"
          />
          <button
            type="button"
            onClick={clear}
            className="rounded-md border border-line bg-surface px-2.5 py-1 text-[12px] text-fg-muted hover:bg-surface-2 hover:text-fg"
          >
            Clear
          </button>
        </div>
      </div>

      {signal === "traces" && <TracesPanel />}
      {signal === "metrics" && <MetricsPanel />}
      {signal === "logs" && <LogsPanel />}
    </div>
  );
}

// ── Logs ────────────────────────────────────────────────────────────────
function LogsPanel() {
  const logs = useTelemetryStore((s) => s.logs);
  const ordered = useMemo(() => logs.slice().reverse(), [logs]);

  if (ordered.length === 0)
    return <Empty hint="host.log.* output streams here, span-correlated." />;

  return (
    <VirtualList
      count={ordered.length}
      rowHeight={28}
      header={
        <Row head>
          <Cell className="w-12">lvl</Cell>
          <Cell className="grow">message</Cell>
          <Cell className="w-24">span</Cell>
        </Row>
      }
      renderRow={(i) => {
        const l = ordered[i]!;
        return (
          <Row className="min-h-7">
            <Cell className="w-12">
              <span className={severityTone(l.severity)}>{l.severity}</span>
            </Cell>
            <Cell className="grow">
              <span className="truncate">{l.body}</span>
            </Cell>
            <Cell className="w-24">
              <span className="text-fg-faint">{l.spanId ? l.spanId.slice(0, 8) : "—"}</span>
            </Cell>
          </Row>
        );
      }}
    />
  );
}

const SEVERITY_TONE: Record<string, string> = {
  ERROR: "text-negative",
  WARN: "text-warning",
  DEBUG: "text-fg-faint",
};

function severityTone(sev: string): string {
  return SEVERITY_TONE[sev] ?? "text-fg-muted";
}

// ── Metrics ─────────────────────────────────────────────────────────────
function MetricsPanel() {
  const metrics = useTelemetryStore((s) => s.metrics);
  const grouped = useMemo(() => groupByName(Object.values(metrics)), [metrics]);

  if (grouped.length === 0)
    return <Empty hint="Interact with the chat — reducer / render timings appear here." />;

  return (
    <div className="flex-1 min-h-0 overflow-y-auto grid gap-4 content-start">
      {grouped.map((g) => (
        <InstrumentSection key={g.name} group={g} />
      ))}
    </div>
  );
}

interface NameGroup {
  name: string;
  unit: string;
  description: string;
  kind: MetricRow["kind"];
  rows: MetricRow[];
}

function groupByName(rows: MetricRow[]): NameGroup[] {
  const by: Record<string, NameGroup> = {};
  for (const r of rows) {
    const g = by[r.name];
    if (g) g.rows.push(r);
    else
      by[r.name] = {
        name: r.name,
        unit: r.unit,
        description: r.description,
        kind: r.kind,
        rows: [r],
      };
  }
  return Object.values(by)
    .map((g) => ({ ...g, rows: g.rows.slice().sort((a, b) => b.count - a.count) }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

function InstrumentSection({ group }: { group: NameGroup }) {
  return (
    <section className="grid gap-1.5">
      <header>
        <div className="font-mono text-[12px] font-semibold text-fg">
          {group.name}
          <span className="ml-2 text-fg-faint">[{group.kind}]</span>
        </div>
        {group.description && (
          <div className="mt-0.5 text-[11.5px] text-fg-muted">{group.description}</div>
        )}
      </header>
      <table className="text-[12px]">
        <thead className="text-[10px] text-fg-faint">
          <tr>
            <th className="py-1 pr-3 text-left font-medium">attrs</th>
            <th className="py-1 pr-3 text-right font-medium">count</th>
            {group.kind === "histogram" && (
              <>
                <th className="py-1 pr-3 text-right font-medium">p50</th>
                <th className="py-1 pr-3 text-right font-medium">p95</th>
                <th className="py-1 pr-3 text-right font-medium">avg</th>
              </>
            )}
            <th className="py-1 pr-3 text-right font-medium">
              {group.kind === "histogram" ? "sum" : "value"}
            </th>
          </tr>
        </thead>
        <tbody className="font-mono">
          {group.rows.map((r) => (
            <tr key={r.id} className="hover:bg-surface-2">
              <td className="py-0.5 pr-3 text-fg-muted">{formatAttrs(r.attrs)}</td>
              <td className="py-0.5 pr-3 text-right tabular-nums text-fg">{r.count}</td>
              {group.kind === "histogram" && (
                <>
                  <td className="py-0.5 pr-3 text-right tabular-nums text-fg">
                    {fmt(r.p50, group.unit)}
                  </td>
                  <td className="py-0.5 pr-3 text-right tabular-nums text-fg">
                    {fmt(r.p95, group.unit)}
                  </td>
                  <td className="py-0.5 pr-3 text-right tabular-nums text-fg">
                    {fmt(r.avg, group.unit)}
                  </td>
                </>
              )}
              <td className="py-0.5 pr-3 text-right tabular-nums text-fg">
                {fmt(r.sum, group.unit)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function formatAttrs(attrs: Record<string, string | number | boolean>): string {
  const entries = Object.entries(attrs);
  if (entries.length === 0) return "—";
  return entries.map(([k, v]) => `${k}=${String(v)}`).join(" ");
}

function fmt(n: number | undefined, unit: string): string {
  if (n === undefined) return "—";
  const rounded = n < 10 ? n.toFixed(1) : Math.round(n).toString();
  return unit ? `${rounded} ${unit}` : rounded;
}
