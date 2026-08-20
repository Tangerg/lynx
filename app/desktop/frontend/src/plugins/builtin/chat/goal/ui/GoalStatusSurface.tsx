import { useRef, useState } from "react";
import { Button, IconButton, TextEditorDialog } from "@/ui";
import { AgentComposerTopTraySurface } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import {
  clearGoal,
  goalCommandWasRetired,
  resumeGoal,
  stopGoal,
  updateGoal,
} from "../application/goalCommands";
import { GOAL_STATUS_I18N, goalCanResume } from "../application/goalStatusPresentation";
import { type GoalReadModel, useGoalMaterial } from "../application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";
import { GoalGlyph } from "./GoalGlyph";

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
  const [pending, setPending] = useState<"clear" | "status" | "edit" | null>(null);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(goal.objective);
  const commandInFlight = useRef(false);
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const canChangeStatus = goal.status === "active" || goalCanResume(goal);
  const canEdit = goal.status !== "completing";
  const nextObjective = draft.trim();
  const canSave = nextObjective.length > 0 && nextObjective !== goal.objective;

  const runCommand = async (
    kind: "clear" | "status" | "edit",
    command: () => Promise<void>,
    fallback: string,
  ) => {
    if (commandInFlight.current || !runtimeCommandsAvailable()) return false;
    commandInFlight.current = true;
    setPending(kind);
    try {
      await command();
      return true;
    } catch (error) {
      if (!goalCommandWasRetired(error)) notifyError(rpcErrorText(error) ?? fallback);
      return false;
    } finally {
      commandInFlight.current = false;
      setPending(null);
    }
  };

  const changeStatus = async () => {
    if (!canChangeStatus) return;
    const fallback = goal.status === "active" ? t("goal.error.pause") : t("goal.error.resume");
    await runCommand(
      "status",
      () => (goal.status === "active" ? stopGoal(goal.sessionId) : resumeGoal(goal.sessionId)),
      fallback,
    );
  };

  const clear = () => runCommand("clear", () => clearGoal(goal.sessionId), t("goal.error.clear"));

  const save = async () => {
    if (!canEdit || !canSave) return;
    const saved = await runCommand(
      "edit",
      () => updateGoal({ sessionId: goal.sessionId, objective: nextObjective }),
      t("goal.error.update"),
    );
    if (saved) setEditing(false);
  };

  const controlsDisabled = pending !== null || !runtimeAvailable;
  const openEditor = () => {
    setDraft(goal.objective);
    setEditing(true);
  };

  return (
    <>
      <div
        data-slot="goal-status-row"
        className="flex w-full items-center justify-between gap-2 px-3 py-1"
      >
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <GoalGlyph className="h-[var(--icon-sm)] w-[var(--icon-sm)] shrink-0 text-fg-faint opacity-70" />
          <Button
            type="button"
            data-slot="goal-summary"
            variant="ghost"
            size="xs"
            press={false}
            disabled={pending !== null || !canEdit}
            className="h-auto min-w-0 flex-1 justify-start rounded-none border-0 p-0 text-ui-sm leading-[max(1rem,1.2em)] font-normal hover:bg-transparent disabled:cursor-default disabled:opacity-100"
            onClick={openEditor}
          >
            <span className="shrink-0 text-fg">{t(GOAL_STATUS_I18N[goal.status].label)}</span>
            <span className="ml-1 min-w-0 truncate text-fg-muted">{goal.objective}</span>
          </Button>
        </div>
        <div data-slot="goal-actions" className="flex shrink-0 items-center gap-2">
          <IconButton
            type="button"
            size="xs"
            iconSize="xs"
            icon="trash"
            quiet
            title={t("goal.action.clear")}
            disabled={controlsDisabled}
            aria-busy={pending === "clear"}
            onClick={() => void clear()}
          />
          {canChangeStatus && (
            <IconButton
              type="button"
              size="xs"
              iconSize="xs"
              icon={goal.status === "active" ? "pause" : "play"}
              quiet
              title={t(goal.status === "active" ? "goal.action.pause" : "goal.action.resume")}
              disabled={controlsDisabled}
              aria-busy={pending === "status"}
              onClick={() => void changeStatus()}
            />
          )}
          {canEdit && (
            <IconButton
              type="button"
              size="xs"
              iconSize="xs"
              icon="edit"
              quiet
              title={t("goal.action.edit")}
              disabled={controlsDisabled}
              onClick={openEditor}
            />
          )}
        </div>
      </div>
      <TextEditorDialog
        open={editing}
        onOpenChange={(open) => {
          if (pending !== "edit") setEditing(open);
        }}
        icon={
          <GoalGlyph
            aria-hidden="true"
            className="h-[var(--icon-lg)] w-[var(--icon-lg)] text-fg-muted"
          />
        }
        title={t("goal.edit.title")}
        closeLabel={t("common.close")}
        label={t("goal.edit.label")}
        value={draft}
        onChange={setDraft}
        cancelLabel={t("common.cancel")}
        saveLabel={t("goal.edit.save")}
        savingLabel={t("goal.edit.saving")}
        busy={pending === "edit"}
        saveDisabled={!canSave}
        onSave={() => void save()}
      />
    </>
  );
}
