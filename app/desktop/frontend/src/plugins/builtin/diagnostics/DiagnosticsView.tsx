// Workspace-view body for the Diagnostics plugin. Reads the local
// ingest store, groups rows by instrument name, and renders one
// section per name. Attribute combinations are listed as sub-rows
// (sorted by count desc so the busiest groups bubble up).
//
// No chart libs — this is dev-time triage, not a dashboard. If the
// numbers ever need a real time-series view, a follow-up plugin can
// register a fancier component for the same instrument set.

import type { MetricRow } from "./store";
import { useMemo } from "react";
import { useDiagnosticsStore } from "./store";

export function DiagnosticsView() {
  const rows = useDiagnosticsStore((s) => s.rows);
  const clear = useDiagnosticsStore((s) => s.clear);

  const grouped = useMemo(() => groupByName(Object.values(rows)), [rows]);

  return (
    <div className="grid h-full gap-4 overflow-y-auto p-6">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-[15px] font-semibold text-fg">Diagnostics</div>
          <div className="mt-0.5 text-[12px] text-fg-muted">
            Live measurements from kernel + plugins. CUMULATIVE since this view
            was mounted; "Clear" resets the in-memory aggregates.
          </div>
        </div>
        <button
          type="button"
          onClick={clear}
          className="rounded-md border border-line bg-surface px-2.5 py-1 text-[12px] text-fg-muted cursor-pointer hover:bg-surface-2 hover:text-fg"
        >
          Clear
        </button>
      </div>

      {grouped.length === 0 ? (
        <div className="text-[13px] text-fg-faint">
          No measurements yet — interact with the chat (send a message, open a
          code block, etc.) and they will appear here.
        </div>
      ) : (
        grouped.map((g) => <InstrumentSection key={g.name} group={g} />)
      )}
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
    if (g) {
      g.rows.push(r);
    } else {
      by[r.name] = {
        name: r.name,
        unit: r.unit,
        description: r.description,
        kind: r.kind,
        rows: [r],
      };
    }
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
        <thead className="text-[10px] uppercase tracking-wider text-fg-faint">
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
                    {fmt(r.last, group.unit)}
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
