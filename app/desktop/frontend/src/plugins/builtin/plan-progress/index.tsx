// Built-in plugin: Plan Progress banner pinned at the top of the
// message stream.
//
// Position history: lived in `chat.status` above the composer (covered
// the tail of the message stream), then briefly as a compact pill in
// the topbar (too peripheral). Final home is `chat.banner.top` — a
// layout slot rendered once above the scrolling message stream. The
// banner is *not* CSS sticky; it just lives outside the message scroll
// container so the chat scrolls beneath it.
//
// Behaviour:
//   - Collapsed (default): one-line summary "Plan · X/Y · pct%" plus
//     the in-flight (or next-todo) task text.
//   - Click anywhere on the body toggles expand: the full plan list
//     renders inline (done / doing / todo glyphs via PlanCheck — same
//     primitives the PlanBlock content-block uses). No navigation —
//     plan info is light enough to read in place.
//   - X on the right dismisses the banner for the current run id;
//     it reappears when a new run starts (runId changes) or a fresh
//     plan ref lands (the reducer's immutable update — agent rebuilt
//     the plan).

import type { PlanItem } from "@/protocol/agui/viewState";
import type { MouseEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useState } from "react";
import { PlanCheck, planItemRow } from "@/components/chat/PlanCheck";
import { Icon, Tooltip } from "@/components/common";
import { swift } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useAgentSlice } from "@/state/agentStore";

function pickCurrent(plan: PlanItem[]): PlanItem | null {
  // Prefer the in-flight task; fall back to the next not-yet-done so
  // the banner always reads "what's happening now".
  return plan.find((p) => p.status === "doing") ?? plan.find((p) => p.status === "todo") ?? null;
}

function PlanProgressBanner() {
  const plan = useAgentSlice((v) => v.plan);
  const runId = useAgentSlice((v) => v.run.runId);
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);

  // Reset both expand + dismiss when a brand-new plan lands. `plan`
  // ref identity follows the reducer's immutable updates — a new
  // array arrives every time the plan content changes, which is
  // exactly when we want to re-surface + re-collapse the banner.
  useEffect(() => {
    setDismissedRunId(null);
    setExpanded(false);
  }, [plan]);

  const hasPlan = plan.some((p) => p.status !== "done");
  const total = plan.length;
  const done = plan.filter((p) => p.status === "done").length;
  const current = pickCurrent(plan);
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  const dismissed = runId !== null && runId === dismissedRunId;
  const visible = hasPlan && current && !dismissed;

  const dismiss = (e: MouseEvent) => {
    // Stop the click from bubbling into the outer toggle button —
    // dismiss shouldn't also expand / collapse.
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
          className="mt-2 mb-1 flex items-start gap-1 rounded-lg border border-line-soft bg-surface px-2 py-2"
        >
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            aria-label={
              expanded ? "Collapse plan list" : `Expand plan (${done}/${total} · ${pct}%)`
            }
            className={cn(
              "min-w-0 flex-1 grid grid-cols-[auto_1fr_auto] items-start gap-2.5",
              "rounded-md border-0 bg-transparent px-1.5 py-0.5",
              "cursor-pointer text-left transition-colors hover:bg-surface-2",
              "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
            )}
          >
            <span className="mt-0.5 grid h-5 w-5 place-items-center rounded-sm bg-surface-2 text-fg-muted">
              <Icon name="list" size={11} />
            </span>
            <div className="min-w-0">
              <div className="flex items-baseline gap-2">
                <span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-fg-faint">
                  Plan · {done}/{total}
                </span>
                <span className="font-mono text-[10px] text-fg-faint">{pct}%</span>
              </div>
              {!expanded && current && (
                <div className="mt-0.5 truncate text-[12.5px] text-fg">{current.text}</div>
              )}
              <AnimatePresence initial={false}>
                {expanded && (
                  <motion.ul
                    key="list"
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: "auto", opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    transition={swift}
                    className="mt-1 overflow-hidden"
                  >
                    {plan.map((p) => (
                      <li key={p.id} className={planItemRow(p.status)}>
                        <PlanCheck status={p.status} />
                        <div>{p.text}</div>
                      </li>
                    ))}
                  </motion.ul>
                )}
              </AnimatePresence>
            </div>
            <Icon
              name={expanded ? "chevron-up" : "chevron-down"}
              size={12}
              className="mt-1 text-fg-faint transition-colors group-hover:text-fg"
            />
          </button>
          <Tooltip label="Dismiss plan">
            <button
              type="button"
              onClick={dismiss}
              aria-label="Dismiss plan banner"
              className={cn(
                "mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-md border-0 bg-transparent",
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
