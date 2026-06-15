// Built-in plugin: the app-bottom status bar — a persistent, full-width
// runtime-observability data row (DESIGN.md §8), contributed to the
// `app.statusbar` kernel slot.
//
// Why a status bar and not the composer footer: the footer is scoped to the
// chat panel, so it vanishes the moment a workspace view (diff / terminal /
// timeline) is promoted, and most of its chips only render while a run is in
// flight. The critical "is the agent alive / how full is the context / what
// has this cost" signals must stay readable everywhere, always — that's what
// this bar owns. The footer keeps the session-context cluster (cwd / branch /
// mode), which is input-adjacent and only relevant while composing.
//
// Items self-omit (return null) when their data is absent; the hairline-pipe
// separators are drawn by CSS between rendered items only (`.sb-item` rule in
// layout.css), so an idle bar reads as just "● idle" with no dangling pipes.

import { useEffect, useMemo, useRef, useState } from "react";
import { Icon, StatusDot, Tooltip } from "@/components/common";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/agent/errorCopy";
import { useActiveSession } from "@/lib/agent/useActiveSession";
import { SESSIONS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { notifyError } from "@/lib/notify";
import { getContainer } from "@/main/container";
import { definePlugin } from "@/plugins/sdk";
import { asSessionId } from "@/rpc";
import {
  useAgentSlice,
  useAgentRunning,
  useAgentRunId,
  useAgentRunTokens,
  useAgentRunCtxPct,
} from "@/state/agentStore";
import { useServerFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

// "1.2k" / "200k" / "1.5M" → number. Conservative; NaN when unparseable so
// callers fall back gracefully.
function parseShorthand(input: string | undefined): number {
  if (!input) return Number.NaN;
  const m = input.trim().match(/^([\d.]+)\s*([km]?)$/i);
  if (!m) return Number.NaN;
  const n = Number.parseFloat(m[1]!);
  const unit = m[2]!.toLowerCase();
  if (unit === "k") return n * 1_000;
  if (unit === "m") return n * 1_000_000;
  return n;
}

const fmtTokens = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 });

// Run state — the leading item, always rendered (it owns the `●`). Narrow
// subscriptions so a token tick doesn't re-render this past the step change.
function RunStatus() {
  const running = useAgentRunning();
  const step = useAgentSlice((v) => v.run.step);
  const totalSteps = useAgentSlice((v) => v.run.totalSteps);
  const activity = useAgentSlice((v) => v.run.activity);
  if (!running) {
    return (
      <span className="sb-item">
        <StatusDot tone="idle" />
        <span className="text-fg-faint">idle</span>
      </span>
    );
  }
  return (
    <span className="sb-item text-accent">
      <StatusDot tone="running" />
      <span>
        {step}/{totalSteps}
      </span>
      <span className="text-fg-faint">·</span>
      <span className="text-fg">{activity || "working"}</span>
    </span>
  );
}

// The active run's id, while it's live — the handle to correlate with backend
// logs / traces. Hidden when idle (a stale id is noise).
function RunId() {
  const running = useAgentRunning();
  const runId = useAgentRunId();
  if (!running || !runId) return null;
  return (
    <Tooltip label={`Run ${runId}`}>
      <span className="sb-item text-fg-faint">
        <span className="max-w-[160px] truncate">{runId}</span>
      </span>
    </Tooltip>
  );
}

// Context-window budget as dense text (DESIGN.md §8: `12,847 / 200k · 6.4%`).
// The status bar has room for the numbers the cramped footer ring hid in a
// tooltip. Persists the last run's values between runs; warning-toned past 90%.
function ContextBudget() {
  const used = useAgentRunTokens().used;
  const total = useAgentSlice((v) => v.run.tokens.total);
  const ctxPct = useAgentRunCtxPct();
  const totalNum = parseShorthand(total);
  if (!Number.isFinite(totalNum) || totalNum <= 0) return null;
  const pct = Math.max(0, Math.min(100, Number(ctxPct) || 0));
  return (
    <Tooltip label={`Context · ${used} / ${total} · ${ctxPct}%`}>
      <span className="sb-item">
        <span className="text-fg-faint">ctx</span>
        <span className={pct >= 90 ? "text-warning" : "text-fg-muted"}>
          {used}/{total} · {ctxPct}%
        </span>
      </span>
    </Tooltip>
  );
}

// One-click context compaction (B10) — summarize earlier messages to reclaim
// room. Surfaces only when the runtime supports it (features.compaction), a
// session is active, and context is filling up (a low-context compact is a
// wasted LLM round-trip). The result streams back as a compaction Item +
// updated usage, so the bar's ctx% falls on its own — no optimistic patching.
function CompactButton() {
  const t = useT();
  const enabled = useServerFeature("compaction");
  const ctxPct = useAgentRunCtxPct();
  const sessionId = useSessionStore((s) => s.activeSessionId);
  const [busy, setBusy] = useState(false);
  if (!enabled || !sessionId || (Number(ctxPct) || 0) < 75) return null;

  const compact = async () => {
    if (busy) return;
    setBusy(true);
    try {
      await getContainer()
        .client()
        .sessions.compact({ sessionId: asSessionId(sessionId) });
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("statusbar.compact.error"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Tooltip label={t("statusbar.compact.label")}>
      <button
        type="button"
        className="sb-item text-warning hover:text-fg disabled:opacity-60"
        onClick={() => void compact()}
        disabled={busy}
      >
        <Icon name="minimize" size={12} />
        <span>{busy ? t("statusbar.compact.busy") : t("statusbar.compact.idle")}</span>
      </button>
    </Tooltip>
  );
}

// TTFT + tokens/sec — measured client-side over the active run.
// Time-to-first-token = elapsed ms from run start to the first non-zero
// `tokens.used` sample; rate = used / (elapsed - TTFT) after a 500ms warm-up
// so the first sample's jitter doesn't blow the number up. Refs (not state)
// for the samples so telemetry ticks don't trigger re-renders just to record.
function Throughput() {
  const running = useAgentRunning();
  const runId = useAgentRunId();
  const used = useAgentSlice((v) => v.run.tokens.used);
  const usedNum = useMemo(() => parseShorthand(used), [used]);

  const startedAtRef = useRef<number | null>(null);
  const ttftMsRef = useRef<number | null>(null);
  const lastRunIdRef = useRef<string | null>(null);
  const [tokensPerSec, setTokensPerSec] = useState<number | null>(null);
  const [ttftMs, setTtftMs] = useState<number | null>(null);

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
    if (sinceFirstToken > 0.5) setTokensPerSec(usedNum / sinceFirstToken);
  }, [usedNum, running]);

  if (!running) return null;
  if (tokensPerSec !== null) {
    return (
      <Tooltip label={`TTFT ${ttftMs?.toFixed(0)}ms · live tokens/sec`}>
        <span className="sb-item text-fg-faint">
          <span>↑</span>
          <span>{tokensPerSec.toFixed(0)} t/s</span>
        </span>
      </Tooltip>
    );
  }
  if (ttftMs !== null) {
    return (
      <Tooltip label="Time to first token">
        <span className="sb-item text-fg-faint">{ttftMs.toFixed(0)}ms</span>
      </Tooltip>
    );
  }
  return (
    <Tooltip label="Waiting for first token…">
      <span className="sb-item text-fg-faint">·</span>
    </Tooltip>
  );
}

// Cumulative session usage (wire Session.usage) — the lifetime "what has this
// conversation cost me" readout. Run-scoped items reset each run; this only
// moves when a run settles, so the running→idle edge refetches the sessions
// list (its 5-minute staleTime is tuned for the sidebar, too slow here).
function SessionCost() {
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
      <span className="sb-item text-fg-faint">
        <span>Σ</span>
        <span>{fmtTokens.format(total)}</span>
        {usage.costUsd !== undefined && <span>${usage.costUsd.toFixed(2)}</span>}
      </span>
    </Tooltip>
  );
}

// Fragment of items — they become direct flex children of the `.statusbar`
// slot wrapper (PluginBoundary + Fragment are DOM-transparent), so the
// `.sb-item` pipe-separator CSS sees them as adjacent siblings.
function StatusBar() {
  return (
    <>
      <RunStatus />
      <RunId />
      <ContextBudget />
      <CompactButton />
      <Throughput />
      <SessionCost />
    </>
  );
}

export const statusBar = definePlugin({
  name: "lyra.builtin.status-bar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.statusbar", { id: "default", order: 0, component: StatusBar });
  },
});
