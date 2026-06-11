// Built-in plugin: run telemetry chips for the composer footer's right
// side. Three glyphs, only while a run matters — run state (live dot +
// step + activity), generation rate (t/s), and a context-usage ring.
// Registered with `align: "end"` so they pin right of the session-context
// chips. Exact numbers live in tooltips; the footer stays light.
//
// Each chip owns its visual treatment via Tailwind utility classes. The
// shared "inline-flex + gap + nowrap" pill shape is captured by the
// `pill` helper below — one internal abstraction to avoid restating the
// same utilities at every site.

import { useEffect, useMemo, useRef, useState } from "react";
import { Icon, StatusDot, Tooltip } from "@/components/common";
import { useActiveSession } from "@/lib/agent/useActiveSession";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { COMPOSER_STATUS } from "@/plugins/sdk/kernelPoints";
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

// Narrow subscriptions: subscribing to the whole `v.run` object would
// re-render this on every telemetry tick (tokens / ctxPct). Pulling each
// primitive via Object.is means RunState skips the renders the rate +
// context ring care about.
//
// Only renders while a run is in flight. Idle state is already conveyed
// by the chat-tab strip, so showing an "idle" pill here would duplicate
// it; collapsing to null keeps the footer quiet between runs.
function RunState() {
  const running = useAgentSlice((v) => v.run.running);
  const step = useAgentSlice((v) => v.run.step);
  const totalSteps = useAgentSlice((v) => v.run.totalSteps);
  const activity = useAgentSlice((v) => v.run.activity);
  const stop = useAgentAction("stop");
  if (!running) return null;
  return (
    <span className={pill("text-accent")}>
      <StatusDot tone="running" />
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
            className="ml-1 inline-flex items-center gap-0.5 rounded-xs border border-line-soft bg-transparent px-1.5 py-px font-mono text-[10px] text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Icon name="stop" size={9} />
            stop
          </button>
        </Tooltip>
      )}
    </span>
  );
}

// Context usage as a single ring — a universal "how full" glyph that
// stays legible at 16px where "142k/200k 9%" would crowd the footer. The
// exact numbers live in the tooltip. Arc turns warning-toned past 90% so
// a near-full context still reads at a glance.
const RING_R = 6;
const RING_C = 2 * Math.PI * RING_R;

function ContextRing() {
  const used = useAgentSlice((v) => v.run.tokens.used);
  const total = useAgentSlice((v) => v.run.tokens.total);
  const ctxPct = useAgentSlice((v) => v.run.ctxPct);
  const pct = Math.max(0, Math.min(100, Number(ctxPct) || 0));
  const stroke = pct >= 90 ? "var(--color-warning)" : "var(--color-accent)";
  return (
    <Tooltip label={`Context · ${ctxPct}% · ${used}/${total}`}>
      <span className={pill()} aria-label={`Context ${ctxPct}% used`}>
        <svg width="16" height="16" viewBox="0 0 16 16" className="-rotate-90">
          <circle
            cx="8"
            cy="8"
            r={RING_R}
            fill="none"
            stroke="var(--color-surface-3)"
            strokeWidth="2"
          />
          <circle
            cx="8"
            cy="8"
            r={RING_R}
            fill="none"
            stroke={stroke}
            strokeWidth="2"
            strokeLinecap="round"
            strokeDasharray={RING_C}
            strokeDashoffset={RING_C * (1 - pct / 100)}
          />
        </svg>
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
// composer footer next to the context ring so the user always sees
// current generation speed regardless of which message they're viewing.
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

// Cumulative session usage (wire Session.usage) — the run-scoped chips
// above reset every run; this is the session's lifetime total, the
// "what has this conversation cost me" readout. Totals only move when a
// run settles, so the running→idle edge refetches the sessions list
// (its 5-minute staleTime is tuned for the sidebar, too slow here).
const fmtTokens = new Intl.NumberFormat("en", {
  notation: "compact",
  maximumFractionDigits: 1,
});

function SessionUsage() {
  const running = useAgentSlice((v) => v.run.running);
  const prevRunning = useRef(running);
  useEffect(() => {
    if (prevRunning.current && !running) {
      void queryClient.invalidateQueries({ queryKey: [SESSIONS_KEY] });
    }
    prevRunning.current = running;
  }, [running]);

  const usage = useActiveSession()?.usage;
  if (!usage) return null;
  const total = (usage.inputTokens ?? 0) + (usage.outputTokens ?? 0);
  if (total === 0 && usage.costUsd === undefined) return null;
  const detail = [
    `in ${fmtTokens.format(usage.inputTokens ?? 0)}`,
    `out ${fmtTokens.format(usage.outputTokens ?? 0)}`,
    ...(usage.costUsd !== undefined ? [`$${usage.costUsd.toFixed(2)}`] : []),
  ].join(" · ");
  return (
    <Tooltip label={`Session total · ${detail}`}>
      <span className={pill("text-fg-faint")}>
        <span>Σ</span>
        <span className="font-mono">{fmtTokens.format(total)}</span>
        {usage.costUsd !== undefined && (
          <span className="font-mono">${usage.costUsd.toFixed(2)}</span>
        )}
      </span>
    </Tooltip>
  );
}

export const statusPill = definePlugin({
  name: "lyra.builtin.status-pill",
  version: "1.0.0",
  setup({ host }) {
    // align: "end" → right cluster of the composer footer, after the
    // session-context chips (exec mode / branch).
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "run",
      order: 90,
      align: "end",
      component: RunState,
    });
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "token-rate",
      order: 91,
      align: "end",
      component: TokenRate,
    });
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "context",
      order: 92,
      align: "end",
      component: ContextRing,
    });
    host.extensions.contribute(COMPOSER_STATUS, {
      id: "session-usage",
      order: 93,
      align: "end",
      component: SessionUsage,
    });
  },
});
