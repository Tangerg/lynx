import type { IconName } from "@/ui";
import { isAgentRunFailure, type AgentRunOutcome } from "@/plugins/builtin/agent/public/viewState";
import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import type { CurrentRootMaterial } from "@/plugins/builtin/agent/public/run";

/**
 * Ordinary completion is already expressed by the final assistant message. Only
 * an exceptional non-failure terminal reason gets a quiet narrative row; errors
 * remain in the actionable recovery surface.
 */
export function RootRunOutcome({ material }: { material: CurrentRootMaterial }) {
  const t = useT();
  const { outcome } = material;
  if (!outcome || outcome.type === "completed" || isAgentRunFailure(outcome)) return null;

  const face = CLOSE_FACE[outcome.type];
  const detail = outcome.detail;

  return (
    <div className="my-2 flex min-w-0 items-center gap-2 text-ui-sm text-fg-faint">
      <Icon name={face.icon} size="xs" className="shrink-0" />
      <span className="shrink-0">{t(face.labelKey)}</span>
      {detail && <span className="min-w-0 truncate">· {detail}</span>}
    </div>
  );
}

const CLOSE_FACE: Record<
  Exclude<AgentRunOutcome["type"], "completed" | "timedOut" | "failed" | "lost">,
  { icon: IconName; labelKey: string }
> = {
  canceled: { icon: "stop", labelKey: "agent.runOutcome.canceled" },
  maxSteps: { icon: "alert", labelKey: "agent.runOutcome.maxSteps" },
  maxBudget: { icon: "alert", labelKey: "agent.runOutcome.maxBudget" },
};
