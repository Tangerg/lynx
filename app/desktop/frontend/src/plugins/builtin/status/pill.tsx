// Built-in plugin: run telemetry chips for the composer footer's right
// side. Bloomberg-style data density — run state with a ticking live dot,
// tokens / cost with mono numbers + a token sparkline tracking how usage
// has grown across the current run. Registered with `align: "end"` so
// they pin to the right of the session-context chips.
//
// Each chip owns its visual treatment via Tailwind utility classes. The
// shared "inline-flex + gap + nowrap" pill shape is captured by the
// `pill` helper below — one internal abstraction to avoid restating the
// same utilities at every site.

import { useEffect, useMemo, useRef, useState } from "react";
import { Icon, Sparkline, StatusDot, Tooltip } from "@/components/common";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentAction, useAgentSlice } from "@/state/agentStore";

// One slot in the status bar. All callers use the same shape:
// inline-flex row, 5px gap, no-wrap, tabular-numeric so digits don't
// shimmy as values tick.
const pill = (extra?: string) =>
  cn("inline-flex items-center gap-1.5 whitespace-nowrap tabular-nums", extra);

// "1.2k" / "200k" / "1.5M" → number. Conservative; if we can't parse,
// return NaN so the caller can fall back gracefully.
function parseShorthand(input: string | undefined): number {
  if (!input) return Number.NaN;
  const m = input.trim().match(/^([\d.]+)\s*([km]?)$/i);
  if (!m) return Number.NaN;
  // Two capture groups in the regex — both defined when match succeeds.
  const n = Number.parseFloat(m[1]!);
  const unit = m[2]!.toLowerCase();
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

// Narrow subscriptions: subscribing to the whole `v.run` object would
// re-render this on every telemetry tick (token / cost / ctxPct). Only
// the four primitives below actually drive the visual; pulling each via
// Object.is means RunState skips renders the other status-bar pieces
// (Tokens / Cost) care about.
function RunState() {
  const running = useAgentSlice((v) => v.run.running);
  const step = useAgentSlice((v) => v.run.step);
  const totalSteps = useAgentSlice((v) => v.run.totalSteps);
  const activity = useAgentSlice((v) => v.run.activity);
  const stop = useAgentAction("stop");
  return (
    <span className={pill(running ? "text-accent" : "")}>
      <StatusDot tone={running ? "running" : "idle"} />
      {running ? (
        <>
          <span className="font-mono">
            {step}/{totalSteps}
          </span>
          <span className="text-fg-faint">·</span>
          <span className="text-fg">{activity || "running"}</span>
          {stop && (
            <Tooltip label="Stop (⌘.)">
              <button
                type="button"
                onClick={stop}
                aria-label="Stop"
                className="ml-1 inline-flex items-center gap-0.5 rounded-xs border border-line-soft bg-transparent px-1.5 py-px font-mono text-[10px] text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
              >
                <Icon name="stop" size={9} />
                stop
              </button>
            </Tooltip>
          )}
        </>
      ) : (
        <span>idle</span>
      )}
    </span>
  );
}

function Tokens() {
  const used = useAgentSlice((v) => v.run.tokens.used);
  const total = useAgentSlice((v) => v.run.tokens.total);
  const ctxPct = useAgentSlice((v) => v.run.ctxPct);
  // parseShorthand walks a regex + does float math — cheap individually
  // but it ran on every render. Memo by the underlying string so it
  // recomputes only when the displayed value actually changes.
  const usedNum = useMemo(() => parseShorthand(used), [used]);
  const history = useNumericHistory(usedNum);
  return (
    <Tooltip label={`Context: ${ctxPct}% of ${total}`}>
      <span className={pill()}>
        <Sparkline values={history} width={42} height={12} fill />
        <span className="font-mono">{used}</span>
        <span className="font-mono text-fg-faint">/{total}</span>
        <span className="font-mono text-fg-faint">{ctxPct}%</span>
      </span>
    </Tooltip>
  );
}

function Cost() {
  const cost = useAgentSlice((v) => v.run.cost);
  return (
    <Tooltip label="Session cost (USD)">
      <span className={pill()}>
        <span className="text-fg-faint">$</span>
        <span className="font-mono">{cost}</span>
      </span>
    </Tooltip>
  );
}

// TTFT + tokens/sec — measured client-side over the active run.
// Time-to-first-token is the elapsed ms from RUN_STARTED to the first
// non-zero `tokens.used` sample we see. Rate is `used / (elapsed - TTFT)`
// computed after a 500ms warm-up so the first sample's jitter doesn't
// blow the number up.
//
// Cherry Studio surfaces this as a per-message badge; we put it in the
// status bar next to Tokens so the user always sees current generation
// speed regardless of which message they're looking at.
function TokenRate() {
  const running = useAgentSlice((v) => v.run.running);
  const runId = useAgentSlice((v) => v.run.runId);
  const used = useAgentSlice((v) => v.run.tokens.used);
  const usedNum = useMemo(() => parseShorthand(used), [used]);

  // Per-run refs reset on RUN_STARTED. Refs (not state) so the rAF-style
  // updates from telemetry don't trigger our own re-renders just to
  // record a sample.
  const startedAtRef = useRef<number | null>(null);
  const ttftMsRef = useRef<number | null>(null);
  const lastRunIdRef = useRef<string | null>(null);
  const [tokensPerSec, setTokensPerSec] = useState<number | null>(null);
  const [ttftMs, setTtftMs] = useState<number | null>(null);

  // Reset on new run.
  useEffect(() => {
    if (!running) {
      startedAtRef.current = null;
      ttftMsRef.current = null;
      lastRunIdRef.current = null;
      setTokensPerSec(null);
      setTtftMs(null);
      return;
    }
    if (runId && runId !== lastRunIdRef.current) {
      lastRunIdRef.current = runId;
      startedAtRef.current = performance.now();
      ttftMsRef.current = null;
      setTokensPerSec(null);
      setTtftMs(null);
    }
  }, [running, runId]);

  // Each token-usage sample: record TTFT on first non-zero, then update
  // rate every sample (cheap — division + setState).
  useEffect(() => {
    if (!running || startedAtRef.current === null) return;
    if (!Number.isFinite(usedNum) || usedNum <= 0) return;
    const elapsed = performance.now() - startedAtRef.current;
    if (ttftMsRef.current === null) {
      ttftMsRef.current = elapsed;
      setTtftMs(elapsed);
      return;
    }
    const sinceFirstToken = (elapsed - ttftMsRef.current) / 1000;
    if (sinceFirstToken > 0.5) {
      setTokensPerSec(usedNum / sinceFirstToken);
    }
  }, [usedNum, running]);

  if (!running) return null;
  if (tokensPerSec !== null) {
    return (
      <Tooltip label={`TTFT ${ttftMs?.toFixed(0)}ms · live tokens/sec`}>
        <span className={pill("text-fg-faint")}>
          <span className="font-mono">{tokensPerSec.toFixed(0)}</span>
          <span>t/s</span>
        </span>
      </Tooltip>
    );
  }
  if (ttftMs !== null) {
    return (
      <Tooltip label="Time to first token">
        <span className={pill("text-fg-faint")}>
          <span className="font-mono">{ttftMs.toFixed(0)}</span>
          <span>ms</span>
        </span>
      </Tooltip>
    );
  }
  return (
    <Tooltip label="Waiting for first token…">
      <span className={pill("text-fg-faint")}>
        <span>·</span>
      </span>
    </Tooltip>
  );
}

export const statusPill = definePlugin({
  name: "lyra.builtin.status-pill",
  version: "1.0.0",
  setup({ host }) {
    // align: "end" → right cluster of the composer footer, after the
    // session-context chips (project / mode / branch).
    host.composer.registerStatus({ id: "run", order: 90, align: "end", component: RunState });
    host.composer.registerStatus({
      id: "token-rate",
      order: 91,
      align: "end",
      component: TokenRate,
    });
    host.composer.registerStatus({ id: "tokens", order: 92, align: "end", component: Tokens });
    host.composer.registerStatus({ id: "cost", order: 93, align: "end", component: Cost });
  },
});
