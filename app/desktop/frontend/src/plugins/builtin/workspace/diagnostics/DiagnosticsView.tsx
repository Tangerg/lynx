// Diagnostics view — renders the local telemetry sink (lib/observability):
// the OTel triad as three tabs. Dev-time triage, not a dashboard.
//
// Perf: telemetry volume is high, so spans/logs are bounded ring buffers in
// the store and rendered with @tanstack/react-virtual (only on-screen rows
// mount). Each panel subscribes to ONLY its signal's slice, so switching
// tabs / a metrics flush never re-renders the trace or log list.

import type { MetricRow, SpanRow } from "@/lib/observability/stores";
import { useTelemetryStore } from "@/lib/observability/stores";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useMemo, useRef, useState } from "react";
import { Segmented } from "@/components/common";

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

// ── Traces ──────────────────────────────────────────────────────────────
function TracesPanel() {
  const spans = useTelemetryStore((s) => s.spans);
  // Newest first. spans changes only once per flush (~500ms) so the reverse
  // copy is cheap and memoized on the (stable-between-flushes) array ref.
  const ordered = useMemo(() => spans.slice().reverse(), [spans]);

  if (ordered.length === 0) return <Empty hint="Send a message — run + RPC spans appear here." />;

  return (
    <VirtualList
      count={ordered.length}
      rowHeight={32}
      header={
        <Row head>
          <Cell className="grow">span</Cell>
          <Cell className="w-16 text-right">dur</Cell>
          <Cell className="w-16">status</Cell>
          <Cell className="w-28">trace</Cell>
        </Row>
      }
      renderRow={(i) => {
        const s = ordered[i]!;
        return (
          <Row>
            <Cell className="grow">
              <span className="truncate">{s.name}</span>
            </Cell>
            <Cell className="w-16 text-right tabular-nums">{s.durationMs.toFixed(1)}ms</Cell>
            <Cell className="w-16">
              <StatusTag status={s.status} />
            </Cell>
            <Cell className="w-28">
              <span className="text-fg-faint">{s.traceId.slice(0, 12)}</span>
            </Cell>
          </Row>
        );
      }}
    />
  );
}

const STATUS_TONE: Record<SpanRow["status"], string> = {
  error: "text-negative",
  ok: "text-positive",
  unset: "text-fg-faint",
};

function StatusTag({ status }: { status: SpanRow["status"] }) {
  return <span className={STATUS_TONE[status]}>{status}</span>;
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
          <Row>
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

// ── Shared list primitives ────────────────────────────────────────────────
function Empty({ hint }: { hint: string }) {
  return <div className="text-[13px] text-fg-faint">No data yet — {hint}</div>;
}

function Row({ children, head }: { children: React.ReactNode; head?: boolean }) {
  return (
    <div
      className={
        "flex items-center gap-3 px-1 font-mono text-[12px] " +
        (head ? "text-[10px] text-fg-faint" : "text-fg hover:bg-surface-2")
      }
    >
      {children}
    </div>
  );
}

function Cell({ className, children }: { className: string; children?: React.ReactNode }) {
  return <div className={`min-w-0 ${className}`}>{children}</div>;
}

// Virtualized fixed-height list — only on-screen rows mount, so a 500-row
// span buffer renders a dozen DOM nodes. `position: absolute` per row is the
// standard react-virtual pattern (allowed by the no-absolute rule for this).
function VirtualList({
  count,
  rowHeight,
  header,
  renderRow,
}: {
  count: number;
  rowHeight: number;
  header: React.ReactNode;
  renderRow: (index: number) => React.ReactNode;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virt = useVirtualizer({
    count,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 12,
  });

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      {header}
      <div ref={parentRef} className="flex-1 min-h-0 overflow-y-auto">
        <div className="relative w-full" style={{ height: virt.getTotalSize() }}>
          {virt.getVirtualItems().map((vi) => (
            <div
              key={vi.key}
              className="absolute left-0 top-0 w-full"
              style={{ height: vi.size, transform: `translateY(${vi.start}px)` }}
            >
              {renderRow(vi.index)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
