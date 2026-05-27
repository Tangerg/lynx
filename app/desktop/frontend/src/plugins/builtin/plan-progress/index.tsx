// Built-in plugin: Plan Progress banner pinned at the top of the
// message stream.
//
// Position history: sat in `chat.status` above the composer (covered
// the tail of the message stream), then briefly as a compact pill in
// the topbar (too peripheral). Final home is `chat.banner.top` — a
// layout slot rendered once above the scrolling message stream. The
// banner is *not* CSS sticky; it just lives outside the message scroll
// container so the chat scrolls beneath it.
//
// Behaviour:
//   - Two-line card: "Plan · X/Y · pct%" + the in-flight (or
//     next-todo) task text.
//   - Click anywhere on the body opens the Plan workspace view.
//   - X button on the right dismisses the banner for the current run
//     id. It reappears when a new run starts (runId changes) or a
//     fresh plan lands (plan ref changes via the reducer's immutable
//     update). So "stream finishes → user closes → next user prompt"
//     re-surfaces the banner automatically.

import type { PlanItem } from "@/protocol/agui/viewState";
import type { MouseEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useState } from "react";
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
  const runId = useAgentSlice((v) => v.run.runId);
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null);

  // Reset the dismissal when a brand-new plan lands. `plan` ref
  // identity follows the reducer's immutable updates — a new array
  // arrives every time the plan content changes, which is exactly
  // when we want to re-surface the banner (new plan from the agent →
  // user hasn't dismissed *this* version yet).
  useEffect(() => {
    setDismissedRunId(null);
  }, [plan]);

  const hasPlan = plan.some((p) => p.status !== "done");
  const total = plan.length;
  const done = plan.filter((p) => p.status === "done").length;
  const current = pickCurrent(plan);
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  const dismissed = runId !== null && runId === dismissedRunId;
  const visible = hasPlan && current && !dismissed;

  const openPlanView = () => {
    useSessionStore.getState().openMainView({ id: "plan", title: "Plan", icon: "list" });
  };

  const dismiss = (e: MouseEvent) => {
    // Stop the click from bubbling into the outer banner button —
    // dismiss shouldn't also open the Plan view.
    e.stopPropagation();
    setDismissedRunId(runId ?? "");
  };

  return (
    <AnimatePresence initial={false}>
      {visible && (
        <motion.div
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={swift}
          className="mt-2 mb-1 flex items-center gap-1 rounded-lg border border-line-soft bg-surface px-2 py-2"
        >
          <button
            type="button"
            onClick={openPlanView}
            aria-label={`Open plan (${done}/${total} · ${pct}%)`}
            className={cn(
              "min-w-0 flex-1 grid grid-cols-[auto_1fr] items-center gap-2.5",
              "rounded-md border-0 bg-transparent px-1.5 py-0.5",
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
          </button>
          <Tooltip label="Dismiss plan">
            <button
              type="button"
              onClick={dismiss}
              aria-label="Dismiss plan banner"
              className={cn(
                "grid h-6 w-6 shrink-0 place-items-center rounded-md border-0 bg-transparent",
                "text-fg-faint cursor-pointer transition-colors",
                "hover:bg-surface-2 hover:text-fg",
                "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
              )}
            >
              <Icon name="x" size={11} />
            </button>
          </Tooltip>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

export default definePlugin({
  name: "lyra.builtin.plan-progress",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.banner.top", {
      id: "plan-progress",
      order: 0,
      component: PlanProgressBanner,
    });
  },
});
