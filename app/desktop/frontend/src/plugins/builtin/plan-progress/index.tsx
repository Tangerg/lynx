// Built-in plugin: floating Plan Progress banner above the composer.
//
// Sits in the `chat.status` slot — a thin strip between the message
// stream and the composer that fades in when the agent has a plan and
// fades out when the plan is empty / fully done. Shows X/Y done, the
// current "doing" task (or first remaining "todo" if none is in
// flight), and click-to-expand-into-Plan-view.
//
// Cherry doesn't have an inline equivalent; Portai's plan-progress-bar
// is the inspiration. The difference is that ours leans on the
// existing Plan workspace view for the full list — the banner is just
// a status surface, not a full management UI.

import type { PlanItem } from "@/protocol/agui/viewState";
import { AnimatePresence, motion } from "motion/react";
import { Icon, Tooltip } from "@/components/common";
import { swift } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentSlice } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

function pickCurrent(plan: PlanItem[]): PlanItem | null {
  // Prefer the in-flight task; fall back to the next not-yet-done so
  // the banner always reads "what's happening now".
  return plan.find((p) => p.status === "doing") ?? plan.find((p) => p.status === "todo") ?? null;
}

function PlanProgressBanner() {
  const plan = useAgentSlice((v) => v.plan);

  // Bail with no DOM if there's nothing planned, or every item is done
  // already — banner shouldn't linger after completion. The
  // AnimatePresence below still gets a graceful exit.
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
        <Tooltip label="Open plan">
          <motion.button
            type="button"
            onClick={openPlanView}
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -2 }}
            transition={swift}
            aria-label="Open plan"
            className={cn(
              "mb-2 grid w-full grid-cols-[auto_1fr_auto] items-center gap-2.5",
              "rounded-lg border border-line-soft bg-surface px-3 py-2",
              "cursor-pointer text-left transition-colors hover:bg-surface-2",
              "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
            )}
          >
            <span className="grid h-5 w-5 place-items-center rounded-sm bg-surface-2 text-fg-muted">
              <Icon name="list" size={11} />
            </span>
            <div className="min-w-0">
              <div className="flex items-baseline gap-2">
                <span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-fg-faint">
                  Plan · {done}/{total}
                </span>
                <span className="font-mono text-[10px] text-fg-faint">{pct}%</span>
              </div>
              <div className="mt-0.5 truncate text-[12.5px] text-fg">{current.text}</div>
            </div>
            <Icon name="more" size={11} className="text-fg-faint -rotate-90" />
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
    host.layout.register("chat.status", {
      id: "plan-progress",
      order: 10,
      component: PlanProgressBanner,
    });
  },
});
