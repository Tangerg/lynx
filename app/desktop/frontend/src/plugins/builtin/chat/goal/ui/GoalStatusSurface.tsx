import type { ComponentPropsWithoutRef } from "react";
import { useRef, useState } from "react";
import { IconButton } from "@/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { goalCommandWasRetired, resumeGoal, stopGoal } from "../application/goalCommands";
import { GOAL_STATUS_I18N, goalCanResume } from "../application/goalStatusPresentation";
import { type GoalReadModel, useGoalMaterial } from "../application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";

/** A Goal is a standing instruction in the composer's attached top tray. Budgets
 * and accounting remain Runtime facts but do not become persistent front-end
 * chrome. */
export function GoalStatusSurface() {
  const material = useGoalMaterial();
  const goal = material.value?.goal;

  if (!goal) return null;
  return (
    <AgentComposerTopTraySurface
      key={JSON.stringify([goal.sessionId, material.generation, goal.createdAt])}
    >
      <GoalRow goal={goal} />
    </AgentComposerTopTraySurface>
  );
}

function GoalRow({ goal }: { goal: GoalReadModel }) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const commandInFlight = useRef(false);
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canChangeStatus = goal.status === "active" || goalCanResume(goal);

  const changeStatus = async () => {
    if (commandInFlight.current || !canChangeStatus || !runtimeCommandsAvailable()) return;
    commandInFlight.current = true;
    setBusy(true);
    try {
      if (goal.status === "active") await stopGoal(goal.sessionId);
      else await resumeGoal(goal.sessionId);
    } catch (error) {
      if (!goalCommandWasRetired(error)) {
        const fallback = goal.status === "active" ? t("goal.error.pause") : t("goal.error.resume");
        notifyError(rpcErrorText(error) ?? fallback);
      }
    } finally {
      commandInFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <div
      data-slot="goal-status-row"
      className="flex w-full items-center justify-between gap-2 px-3 py-[5px]"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <GoalGlyph className="h-[var(--icon-sm)] w-[var(--icon-sm)] shrink-0 text-fg-faint opacity-70" />
        <span className="flex min-w-0 flex-1 items-center overflow-hidden text-ui-sm leading-tight">
          <span className="shrink-0 text-fg">{t(GOAL_STATUS_I18N[goal.status].label)}</span>
          <span className="ml-1 min-w-0 truncate text-fg-muted">{goal.objective}</span>
        </span>
      </div>
      {canChangeStatus && (
        <IconButton
          type="button"
          size="xs"
          iconSize="sm"
          icon={goal.status === "active" ? "pause" : "play"}
          quiet
          title={t(goal.status === "active" ? "goal.action.pause" : "goal.action.resume")}
          disabled={busy || !runtimeAvailable}
          aria-busy={busy}
          onClick={() => void changeStatus()}
        />
      )}
    </div>
  );
}

function GoalGlyph({ className, ...props }: ComponentPropsWithoutRef<"svg">) {
  return (
    <svg
      {...props}
      data-slot="goal-glyph"
      aria-hidden="true"
      className={className}
      viewBox="0 0 20 20"
      fill="none"
    >
      <path
        d="M9.96861 1.91681C10.3002 1.91681 10.569 2.18564 10.569 2.51722C10.5688 2.84865 10.3001 3.11764 9.96861 3.11764C6.14529 3.11779 3.04595 6.21713 3.04579 10.0404C3.04597 13.8637 6.14531 16.964 9.96861 16.9641C13.792 16.9641 16.8921 13.8638 16.8923 10.0404C16.8925 9.709 17.1612 9.44003 17.4927 9.44003C17.8241 9.44019 18.093 9.7091 18.0931 10.0404C18.0929 14.527 14.4552 18.165 9.96861 18.165C5.48215 18.1648 1.84515 14.5269 1.84497 10.0404C1.84513 5.55398 5.48214 1.91697 9.96861 1.91681Z"
        fill="currentColor"
      />
      <path
        d="M8.73428 5.4417C9.05275 5.34987 9.38553 5.53321 9.47752 5.85167C9.56932 6.17 9.38575 6.50275 9.06755 6.59491C7.60672 7.01688 6.53899 8.36477 6.53894 9.96021C6.53907 11.8943 8.10685 13.4629 10.0409 13.4631C11.6106 13.463 12.9407 12.429 13.385 11.0041C13.4838 10.6877 13.8206 10.5114 14.1371 10.61C14.4536 10.7087 14.6308 11.0455 14.5321 11.3621C13.9357 13.2742 12.1509 14.663 10.0409 14.663C7.44369 14.6628 5.33824 12.5574 5.33812 9.96021C5.33816 7.81571 6.77345 6.00809 8.73428 5.4417Z"
        fill="currentColor"
      />
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M13.8656 1.99087C14.3948 1.60393 15.1805 1.97721 15.1739 2.67063L15.1528 4.83776L17.319 4.8166L17.4539 4.82541C18.1023 4.92002 18.4014 5.73603 17.9115 6.22638L15.5046 8.63331C15.3075 8.83039 15.04 8.94171 14.7613 8.94189H12.2063L10.3936 10.7555C10.1591 10.9899 9.77811 10.9899 9.54364 10.7555C9.30989 10.521 9.30952 10.1407 9.54364 9.90643L11.0486 8.40144V5.22922C11.0486 4.95027 11.1591 4.68234 11.3563 4.48509L13.7633 2.07816L13.8656 1.99087ZM12.2495 5.29005V7.74107H14.6978L16.4136 6.02536L13.9414 6.05004L13.9643 3.57434L12.2495 5.29005Z"
        fill="currentColor"
      />
    </svg>
  );
}
