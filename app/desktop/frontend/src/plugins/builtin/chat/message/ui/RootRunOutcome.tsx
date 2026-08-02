import { Divider, Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { useCurrentRootOutcome } from "@/plugins/builtin/agent/public/run";

/**
 * Quiet terminal marker for the latest root Run.
 *
 * Errors live in the actionable recovery banner, so this row handles only
 * successful, canceled and limit terminal outcomes. Keeping it inside the
 * transcript gives the user a stable end-of-Run fact without turning the
 * header or the whole reading surface into a status display.
 */
export function RootRunOutcome() {
  const t = useT();
  const outcome = useCurrentRootOutcome();
  if (!outcome || outcome.type === "error") return null;

  if (outcome.type === "completed") {
    return (
      <Divider icon={<Icon name="check" size="xs" />} intent="accent">
        {t("agent.runOutcome.completed")}
      </Divider>
    );
  }

  if (outcome.type === "canceled") {
    return (
      <Divider icon={<Icon name="stop" size="xs" />}>
        <span>
          {t("agent.runOutcome.canceled")}
          {outcome.detail && <span className="font-normal"> · {outcome.detail}</span>}
        </span>
      </Divider>
    );
  }

  return (
    <Divider icon={<Icon name="alert" size="xs" />}>
      <span>
        {t(
          outcome.type === "maxSteps" ? "agent.runOutcome.maxSteps" : "agent.runOutcome.maxBudget",
        )}
        {outcome.detail && <span className="font-normal"> · {outcome.detail}</span>}
      </span>
    </Divider>
  );
}
