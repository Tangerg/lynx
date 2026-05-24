// Built-in plugin: status-bar items. Direction 2 — Bloomberg-style data
// density. Run state on the left with a ticking live dot; tokens / cost
// pinned right with mono numbers + a token sparkline tracking how usage
// has grown across the current run.
//
// Each pill component now owns its visual treatment via Tailwind utility
// classes (P6.4 migration). The shared "inline-flex + gap + nowrap"
// status-bar pill shape is captured by the `pill` helper below — one
// internal abstraction to avoid restating the same 4 utilities at every
// pill site.

import { useEffect, useRef, useState } from "react";
import { Icon, Sparkline, StatusDot } from "@/components/common";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentAction, useAgentSlice } from "@/state/agentStore";

// One slot in the status bar. All callers use the same shape:
// inline-flex row, 5px gap, no-wrap, tabular-numeric so digits don't
// shimmy as values tick.
const pill = (extra?: string) =>
  cn(
    "inline-flex items-center gap-1.5 whitespace-nowrap tabular-nums",
    extra,
  );

// "1.2k" / "200k" / "1.5M" → number. Conservative; if we can't parse,
// return NaN so the caller can fall back gracefully.
function parseShorthand(input: string | undefined): number {
  if (!input) return NaN;
  const m = input.trim().match(/^([\d.]+)\s*([kmKM]?)$/);
  if (!m) return NaN;
  const n = parseFloat(m[1]);
  const unit = m[2].toLowerCase();
  if (unit === "k") return n * 1_000;
  if (unit === "m") return n * 1_000_000;
  return n;
}

// Push a fresh sample whenever the underlying value moves, capped at
// MAX so the sparkline buffer doesn't grow forever during long runs.
function useNumericHistory(current: number, max = 32): number[] {
  const [history, setHistory] = useState<number[]>([]);
  const lastRef = useRef<number | null>(null);
  useEffect(() => {
    if (Number.isNaN(current)) return;
    if (lastRef.current === current) return;
    lastRef.current = current;
    setHistory((h) => (h.length >= max ? [...h.slice(1), current] : [...h, current]));
  }, [current, max]);
  return history;
}

function RunState() {
  const run = useAgentSlice((v) => v.run);
  const stop = useAgentAction("stop");
  return (
    <span className={pill(run.running ? "text-accent" : "")}>
      <StatusDot tone={run.running ? "running" : "idle"} />
      {run.running ? (
        <>
          <span className="font-mono">{run.step}/{run.totalSteps}</span>
          <span className="text-fg-faint">·</span>
          <span className="text-fg">{run.activity || "running"}</span>
          {stop && (
            <button
              type="button"
              onClick={stop}
              title="Stop (⌘.)"
              className="ml-1 inline-flex items-center gap-0.5 rounded-xs border border-line-soft bg-transparent px-1.5 py-px font-mono text-[10px] text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
            >
              <Icon name="stop" size={9} />stop
            </button>
          )}
        </>
      ) : (
        <span>idle</span>
      )}
    </span>
  );
}

// Branch indicator. Currently a static "main" placeholder — wiring a
// real git branch needs a Go-side gateway (the renderer can't shell
// out). Until that lands, the chip exists so the slot is reserved and
// the visual density is honest.
function Branch() {
  return (
    <span className={pill()} title="Git branch (placeholder)">
      <Icon name="branch" size={10} className="text-fg-faint" />
      <span className="font-mono">main</span>
    </span>
  );
}

// RunId — 8-char prefix of the active run's id. Mono + tnum so the
// glyphs line up. Becomes "—" between runs.
function RunId() {
  const runId = useAgentSlice((v) => v.run.runId);
  const short = runId ? runId.slice(0, 8) : "—";
  return (
    <span className={pill()} title={runId ? `Run: ${runId}` : "No active run"}>
      <span className="text-fg-faint">run</span>
      <span className="font-mono">{short}</span>
    </span>
  );
}

function Spacer() { return <span className="flex-1" />; }

function Tokens() {
  const run = useAgentSlice((v) => v.run);
  const usedNum = parseShorthand(run.tokens.used);
  const history = useNumericHistory(usedNum);
  return (
    <span className={pill()} title={`Context: ${run.ctxPct}% of ${run.tokens.total}`}>
      <Sparkline values={history} width={42} height={12} fill />
      <span className="font-mono">{run.tokens.used}</span>
      <span className="font-mono text-fg-faint">/{run.tokens.total}</span>
      <span className="font-mono text-fg-faint">{run.ctxPct}%</span>
    </span>
  );
}

function Cost() {
  const run = useAgentSlice((v) => v.run);
  return (
    <span className={pill()} title="Session cost (USD)">
      <span className="text-fg-faint">$</span>
      <span className="font-mono">{run.cost}</span>
    </span>
  );
}

export const statusPill = definePlugin({
  name: "lyra.builtin.status-pill",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.statusbar", { id: "run",     order: 0,   component: RunState });
    host.layout.register("app.statusbar", { id: "branch",  order: 10,  component: Branch });
    host.layout.register("app.statusbar", { id: "runid",   order: 20,  component: RunId });
    host.layout.register("app.statusbar", { id: "spacer",  order: 100, component: Spacer });
    host.layout.register("app.statusbar", { id: "tokens",  order: 200, component: Tokens });
    host.layout.register("app.statusbar", { id: "cost",    order: 210, component: Cost });
  },
});
