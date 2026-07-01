// Built-in plugin: Plan Progress banner pinned at the top of the
// message stream. A single-row header that morphs between "current
// task" (collapsed) and "N tasks · pct% complete" (expanded), so the
// user always sees the same vertical rhythm whether the body is open
// or shut.
//
// Layout rhythm (single shared grid template across header + body
// rows so icons line up vertically — the previous design had a
// 2-line eyebrow header that the list items couldn't align to):
//
//   grid-template-columns: 18px 1fr auto auto auto
//                          ↑    ↑   ↑    ↑    ↑
//                          icon text %    ▼    ×
//
//   header row: status icon · summary text · percent · chevron · X
//   list rows:  per-item    · text         ·  —      ·  —      · —
//
// Behaviour:
//   - Click anywhere on the body toggles expand inline (no nav).
//   - X dismisses for the current run id; reappears when a fresh
//     plan ref lands (reducer's immutable update) or a new run starts.

import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import type { MouseEvent } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useEffect, useState } from "react";
import { PlanCheck } from "@/plugins/builtin/agent/public/planPresentation";
import { Icon, Tooltip } from "@/components/common";
import { swift } from "@/lib/motion";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useActiveRunId, useActiveRunPlan } from "@/plugins/builtin/agent/public/run";
import { definePlugin } from "@/plugins/sdk";

function pickCurrent(plan: PlanItem[]): PlanItem | null {
  // Prefer the in-flight task; fall back to the next not-yet-done so
  // the header always reads "what's happening now".
  return plan.find((p) => p.status === "doing") ?? plan.find((p) => p.status === "todo") ?? null;
}

function PlanProgressBanner() {
  const t = useT();
  const plan = useActiveRunPlan();
  const runId = useActiveRunId();
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);

  // Collapse the banner when a NEW run starts — NOT on every plan change. The
  // reducer creates a fresh plan array on each step-status tick, so a [plan] dep
  // fired every tick: a user who expanded the list got it snapped shut (or a
  // dismissed banner reappeared) the moment the agent flipped a step. Dismiss is
  // already run-scoped via `runId === dismissedRunId`, so a new run re-surfaces
  // it without an explicit reset here.
  useEffect(() => {
    setExpanded(false);
  }, [runId]);

  const hasPlan = plan.some((p) => p.status !== "done");
  const total = plan.length;
  const done = plan.filter((p) => p.status === "done").length;
  const current = pickCurrent(plan);
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  const dismissed = runId !== null && runId === dismissedRunId;
  const visible = hasPlan && current && !dismissed;

  const dismiss = (e: MouseEvent) => {
    // Stop the click from bubbling into the outer toggle button —
    // dismiss shouldn't also flip expand.
    e.stopPropagation();
    setDismissedRunId(runId ?? "");
  };

  // Per-row text colour for the expanded list. `text-line-through`
  // is conditional on done. Mirrors the PlanBlock content-block.
  const itemTextClass = (status: PlanItem["status"]) =>
    cn(
      "flex-1 min-w-0 text-[13px] leading-[1.5] truncate",
      status === "done" && "text-fg-faint line-through decoration-line-soft",
      status === "doing" && "text-fg font-semibold",
      status === "todo" && "text-fg-soft",
    );

  return (
    <AnimatePresence initial={false}>
      {visible && (
        <motion.div
          initial={{ opacity: 0, y: -4 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -4 }}
          transition={swift}
          className="mt-2 mb-1 rounded-lg bg-surface overflow-hidden shadow-[var(--shadow-surface)]"
        >
          {/* Header row — single line, fixed height. Clickable area
              spans icon+text+percent+chevron; X sits outside it so the
              dismiss click doesn't toggle expand. */}
          <div className="flex items-center">
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              aria-expanded={expanded}
              aria-label={expanded ? t("plan.collapse") : t("plan.expand", { done, total, pct })}
              className={cn(
                "flex-1 min-w-0 flex items-center gap-2.5 px-3 py-2.5",
                "border-0 bg-transparent text-left transition-colors hover:bg-surface-2",
                "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-accent",
              )}
            >
              {/* Status indicator — uses the current task's status so
                  the header glyph matches what the list shows for the
                  active row. */}
              <PlanCheck status={current.status} />
              {/* Summary text — switches between "current task" and
                  "N done of M" when expanded. AnimatePresence mode=
                  "wait" cross-fades the two states cleanly. */}
              <AnimatePresence mode="wait" initial={false}>
                <motion.span
                  key={expanded ? "summary" : "current"}
                  initial={{ opacity: 0, y: 3 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -3 }}
                  transition={swift}
                  className="flex-1 min-w-0 truncate text-[13px] leading-[1.4] text-fg"
                >
                  {expanded ? t("plan.complete", { done, total }) : current.text}
                </motion.span>
              </AnimatePresence>
              <span className="shrink-0 font-mono text-[11px] font-medium text-fg-muted">
                {pct}%
              </span>
              <Icon
                name={expanded ? "chevron-up" : "chevron-down"}
                size={14}
                className="shrink-0 text-fg-faint"
              />
            </button>
            <Tooltip label={t("plan.dismiss")}>
              <button
                type="button"
                onClick={dismiss}
                aria-label={t("plan.dismissAria")}
                className={cn(
                  "mr-1.5 grid h-7 w-7 shrink-0 place-items-center rounded-md border-0 bg-transparent",
                  "text-fg-faint transition-colors",
                  "hover:bg-surface-2 hover:text-fg",
                  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent",
                )}
              >
                <Icon name="x" size={12} />
              </button>
            </Tooltip>
          </div>

          {/* Body — same horizontal padding as the header so item
              icons (18px PlanCheck) sit in the same column as the
              header status indicator. CSS grid-rows trick gives a
              smooth height transition without measuring layout. */}
          <div
            className={cn(
              "grid transition-[grid-template-rows] duration-150 ease-out",
              expanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
            )}
          >
            <div className="overflow-hidden">
              <ul className="flex flex-col gap-1 px-3 py-2">
                {plan.map((p) => (
                  <li key={p.id} className="flex items-center gap-2.5 py-0.5">
                    <PlanCheck status={p.status} />
                    <span className={itemTextClass(p.status)}>{p.text}</span>
                  </li>
                ))}
              </ul>
            </div>
          </div>
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
