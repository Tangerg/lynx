import { useEffect, useSyncExternalStore } from "react";
import { Button, ConfirmDialog } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { useGoalMaterial } from "../application/goalReadModel";
import { GoalComposerModeOwner } from "../application/goalComposerMode";
import { goalCanEnterComposerMode } from "../application/goalComposerSubmitMode";
import { GoalGlyph } from "./GoalGlyph";

export function GoalModeIndicator() {
  const sessionId = useActiveSessionId();
  if (!sessionId) return null;
  return <SessionGoalModeIndicator key={sessionId} sessionId={sessionId} />;
}

function SessionGoalModeIndicator({ sessionId }: { sessionId: string }) {
  const t = useT();
  const owner = GoalComposerModeOwner.current();
  const snapshot = useSyncExternalStore(owner.subscribe, owner.snapshot, owner.snapshot);
  const material = useGoalMaterial().value;
  const goal = material?.goal ?? null;
  const available = material?.available === true && goalCanEnterComposerMode(goal);
  const active = snapshot.sessionId === sessionId && snapshot.phase !== "inactive";

  useEffect(
    () => () => {
      owner.deactivate(sessionId);
    },
    [owner, sessionId],
  );

  useEffect(() => {
    if (!available && snapshot.sessionId === sessionId && snapshot.phase !== "starting") {
      owner.deactivate(sessionId);
    }
  }, [available, owner, sessionId, snapshot.phase, snapshot.sessionId]);

  if (!active) return null;
  const confirming = snapshot.phase === "confirming";
  const starting = snapshot.phase === "starting";

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="md"
        press={false}
        aria-label={t("goal.mode.clear")}
        aria-pressed="true"
        disabled={starting}
        className="gap-1.5 px-2 text-ui-sm text-fg-soft hover:bg-hover hover:text-fg"
        onClick={() => owner.deactivate(sessionId)}
      >
        <GoalGlyph className="size-[var(--icon-sm)] shrink-0 opacity-70" />
        <span>{t("goal.mode.label")}</span>
      </Button>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!open) owner.cancelReplacement(sessionId);
        }}
        title={t("goal.replace.title")}
        body={t("goal.replace.body", { objective: snapshot.replacedObjective ?? "" })}
        confirmLabel={t("goal.replace.confirm")}
        cancelLabel={t("common.cancel")}
        destructive
        onConfirm={() => owner.confirmReplacement(sessionId)}
      />
    </>
  );
}
