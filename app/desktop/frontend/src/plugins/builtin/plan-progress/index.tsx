// Built-in plugin: compact Plan Progress pill in the chat top bar.
//
// Used to sit in the `chat.status` slot — a full-width 2-line banner
// between the message stream and the composer. That position covered
// the last visible message on shorter viewports, so the indicator
// moved up to `chat.topbar.actions` (next to the new-tab button) as
// a single-line pill: "Plan · 4/7" + a tiny progress bar. Hover
// surfaces the current task's full text via the shared Tooltip; click
// opens the Plan workspace view.
//
// Cherry doesn't have an inline equivalent; Portai's plan-progress-bar
// is the original inspiration. The difference is that ours leans on
// the existing Plan workspace view for the full list — the pill is
// just a status surface, not a full management UI.

import type { PlanItem } from "@/protocol/agui/viewState";
import { AnimatePresence, motion } from "motion/react";
import { Icon, Tooltip } from "@/components/common";
import { swift } from "@/lib/motion";
import { definePlugin } from "@/plugins/sdk";
import { useAgentSlice } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

function pickCurrent(plan: PlanItem[]): PlanItem | null {
  // Prefer the in-flight task; fall back to the next not-yet-done so
  // the pill always reads "what's happening now".
  return plan.find((p) => p.status === "doing") ?? plan.find((p) => p.status === "todo") ?? null;
}

function PlanProgressPill() {
  const plan = useAgentSlice((v) => v.plan);

  // Bail with no DOM if there's nothing planned, or every item is done
  // already — pill shouldn't linger after completion. AnimatePresence
  // gives it a graceful exit.
  // (`Array#some` returns false on an empty array, so we don't need an
  // extra length check.)
  const hasPlan = plan.some((p) => p.status !== "done");

  const total = plan.length;
  const done = plan.filter((p) => p.status === "done").length;
  const current = pickCurrent(plan);
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;

  const openPlanView = () => {
    useSessionStore.getState().openMainView({ id: "plan", title: "Plan", icon: "list" });
  };

  return (
    <AnimatePresence initial={false}>
      {hasPlan && current && (
        <Tooltip label={`${current.text} · ${pct}%`} side="bottom">
          <motion.button
            type="button"
            onClick={openPlanView}
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            transition={swift}
            aria-label={`Open plan (${done}/${total})`}
            className="ml-1 mr-1 mb-1 inline-flex h-6.5 items-center gap-1.5 rounded-md border border-line-soft bg-transparent px-2 font-mono text-[11px] font-semibold text-fg-muted whitespace-nowrap cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Icon name="list" size={11} className="text-fg-faint" />
            <span className="text-fg-faint">Plan</span>
            <span className="text-fg-faint">·</span>
            <span className="tabular-nums text-fg">
              {done}/{total}
            </span>
            {/* Tiny progress bar — fixed 32px wide, accent fill scales
                to %. Stays in the typography baseline (h-1, not a ring)
                so the pill doesn't grow taller than the surrounding
                topbar buttons. */}
            <span className="ml-0.5 inline-block h-1 w-8 overflow-hidden rounded-full bg-line">
              <span
                className="block h-full rounded-full bg-accent transition-[width] duration-300 ease-out"
                style={{ width: `${pct}%` }}
              />
            </span>
          </motion.button>
        </Tooltip>
      )}
    </AnimatePresence>
  );
}

export default definePlugin({
  name: "lyra.builtin.plan-progress",
  version: "1.0.0",
  setup({ host }) {
    // Live in the topbar action row (alongside the new-tab "+" button)
    // rather than `chat.status` above the composer — the old position
    // obscured the tail of the message stream.
    host.layout.register("chat.topbar.actions", {
      id: "plan-progress",
      order: -10,
      component: PlanProgressPill,
    });
  },
});
