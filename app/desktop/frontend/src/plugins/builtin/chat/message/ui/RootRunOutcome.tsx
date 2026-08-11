import type { IconName } from "@/ui";
import { isAgentRunFailure, type AgentRunOutcome } from "@/plugins/builtin/agent/public/viewState";
import { Divider, Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { useCurrentRootMetrics, useCurrentRootOutcome } from "@/plugins/builtin/agent/public/run";
import { runCloseReadout, type RunCloseReadout } from "../application/runCloseModel";

/**
 * How the latest root Run ended, and what it cost.
 *
 * Errors live in the actionable recovery banner, so this handles only the outcomes a
 * reader can do nothing about: success, cancellation, and the two limits. Keeping it
 * inside the transcript gives the turn a close — a stable end-of-Run fact — without
 * turning the header or the reading surface into a status display.
 *
 * The figures are the point of the row. A turn that ended with the single word
 * "Completed" was the flattest moment on the page, and the run already knew how long
 * it took, how many steps it spent and what it billed.
 */
export function RootRunOutcome() {
  const t = useT();
  const outcome = useCurrentRootOutcome();
  const metrics = useCurrentRootMetrics();
  if (!outcome || isAgentRunFailure(outcome)) return null;

  const face = CLOSE_FACE[outcome.type];
  const detail = outcome.type === "completed" ? undefined : outcome.detail;
  const close = runCloseReadout(metrics);

  return (
    <Divider icon={<Icon name={face.icon} size="xs" />} intent={face.intent}>
      <span className="flex min-w-0 items-baseline gap-1.5">
        <span className="shrink-0">{t(face.labelKey)}</span>
        {detail && <span className="min-w-0 truncate font-normal">· {detail}</span>}
        {close && <RunCloseFigures close={close} />}
      </span>
    </Divider>
  );
}

/**
 * The cost figures, in the readout grammar the usage chip already established
 * (`↑` in, `↓` out) so the app has one spelling for a token pair rather than two.
 * Hidden on a narrow pane for the same reason the tool row's counts are: the words
 * are the fact, the figures are the footnote.
 */
function RunCloseFigures({ close }: { close: RunCloseReadout }) {
  const t = useT();
  return (
    <span className="hidden shrink-0 items-baseline gap-1.5 font-mono font-normal tabular-nums sm:flex">
      {close.duration && <span>· {close.duration}</span>}
      {close.steps !== undefined && <span>· {t("agent.steps", { count: close.steps })}</span>}
      {close.inputTokens && <span>· ↑{close.inputTokens}</span>}
      {close.outputTokens && <span>↓{close.outputTokens}</span>}
      {close.cost && <span>· {close.cost}</span>}
    </span>
  );
}

const CLOSE_FACE: Record<
  Exclude<AgentRunOutcome["type"], "timedOut" | "failed" | "lost">,
  { icon: IconName; intent: "neutral" | "accent"; labelKey: string }
> = {
  completed: { icon: "check", intent: "accent", labelKey: "agent.runOutcome.completed" },
  canceled: { icon: "stop", intent: "neutral", labelKey: "agent.runOutcome.canceled" },
  maxSteps: { icon: "alert", intent: "neutral", labelKey: "agent.runOutcome.maxSteps" },
  maxBudget: { icon: "alert", intent: "neutral", labelKey: "agent.runOutcome.maxBudget" },
};
