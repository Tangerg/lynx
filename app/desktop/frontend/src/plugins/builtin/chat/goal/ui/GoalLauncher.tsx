import { useEffect, useRef, useState } from "react";
import { ConfirmDialog, IconButton, PillButton, Popover, TextArea, TextField } from "@/ui";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { getActiveSessionId, useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import {
  getComposerText,
  useComposerText,
  useSetComposerText,
} from "@/plugins/builtin/chat/composer/public/draft";
import { useComposerModelPreference } from "@/plugins/builtin/chat/composer/public/modelPreference";
import { useRuntimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { onGoalLauncherRequest } from "../adapters/goalLauncherRequest";
import { goalCommandWasRetired, startGoal } from "../application/goalCommands";
import {
  parseGoalStartDraft,
  type GoalStartDraft,
  type GoalStartDraftField,
} from "../application/goalStartDraft";
import { useGoalMaterial, type GoalReadModel } from "../application/goalReadModel";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";

const EMPTY_LIMITS = { maxRuns: "", maxCostUsd: "", maxSteps: "" } as const;

export function GoalLauncher() {
  const sessionId = useActiveSessionId();
  return sessionId ? <SessionGoalLauncher key={sessionId} sessionId={sessionId} /> : null;
}

/**
 * Whether a goal can be set here at all, and nothing else.
 *
 * Split from the panel so that being mounted IS the answer. `/goal` reports
 * failure by finding no listener (see `requestGoalLauncher`), and that only tells
 * the truth if the subscription lives on the far side of this gate — hooks run
 * before an early return, so a panel that gated itself would have gone on
 * listening from behind its own `null`.
 */
function SessionGoalLauncher({ sessionId }: { sessionId: string }) {
  const goalsAvailable = useRuntimeCapability("goals");
  const data = useGoalMaterial().value;
  const existing = data?.goal ?? null;
  const replaceable = !existing || existing.status === "paused" || existing.status === "blocked";
  if (!goalsAvailable || data?.available !== true || !replaceable) return null;
  return <GoalLauncherPanel sessionId={sessionId} existing={existing} />;
}

function GoalLauncherPanel({
  sessionId,
  existing,
}: {
  sessionId: string;
  /** The goal this one would replace, when there is one to replace. */
  existing: GoalReadModel | null;
}) {
  const t = useT();
  const composerText = useComposerText();
  const setComposerText = useSetComposerText();
  const { provider, model } = useComposerModelPreference();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [replacing, setReplacing] = useState(false);
  const [invalid, setInvalid] = useState<GoalStartDraftField | null>(null);
  const [draft, setDraft] = useState<GoalStartDraft>({
    objective: "",
    ...EMPTY_LIMITS,
  });
  const mounted = useRef(true);
  useEffect(
    () => () => {
      mounted.current = false;
    },
    [],
  );

  // `/goal <objective>` lands here. It seeds the draft directly rather than going
  // through the composer, because the composer is where the command's own text was
  // typed and echoing it back would leave `/goal` in the objective.
  useEffect(
    () =>
      onGoalLauncherRequest((objective) => {
        setDraft({ objective, ...EMPTY_LIMITS });
        setInvalid(null);
        setOpen(true);
      }),
    [],
  );

  const update = (field: GoalStartDraftField, value: string) => {
    setDraft((current) => ({ ...current, [field]: value }));
    if (invalid === field) setInvalid(null);
  };

  // The confirmation is a modal over this popover, and dismissing the popover
  // underneath it would take the draft with it — so while the question is open,
  // the panel behind it declines to close.
  const changeOpen = (next: boolean) => {
    if (busy || replacing) return;
    setOpen(next);
    setInvalid(null);
    if (next) setDraft({ objective: composerText, ...EMPTY_LIMITS });
  };

  /**
   * Validate, then ask before overwriting.
   *
   * Validation runs FIRST so a malformed limit is answered where it was typed,
   * rather than after a dialog the user then has to back out of.
   */
  const submit = () => {
    if (busy || !runtimeCommandsAvailable()) return;
    const parsed = parseGoalStartDraft(draft);
    if (!parsed.ok) {
      setInvalid(parsed.field);
      return;
    }
    // Starting one goal while another is on the books ENDS the other, along with
    // everything it has already spent, and the runtime keeps no copy. That is
    // reachable here because the launcher stays available for a paused or blocked
    // goal — which is the case where a user is most likely to be reaching for it
    // to adjust, not to discard.
    if (existing) {
      setReplacing(true);
      return;
    }
    void start();
  };

  const start = async () => {
    const parsed = parseGoalStartDraft(draft);
    if (!parsed.ok) {
      setInvalid(parsed.field);
      return;
    }

    setBusy(true);
    try {
      await startGoal({
        sessionId,
        objective: parsed.objective,
        ...(provider && model ? { provider, model } : {}),
        ...(parsed.budget ? { budget: parsed.budget } : {}),
      });
      if (!mounted.current || getActiveSessionId() !== sessionId) return;
      setOpen(false);
      if (getComposerText().trim() === parsed.objective) setComposerText("");
    } catch (error) {
      if (!goalCommandWasRetired(error) && mounted.current && getActiveSessionId() === sessionId) {
        notifyError(rpcErrorText(error) ?? t("goal.error.start"));
      }
    } finally {
      if (mounted.current) setBusy(false);
    }
  };

  return (
    <>
      <Popover.Root open={open} onOpenChange={changeOpen}>
        <Popover.Trigger
          render={
            <IconButton
              icon="target"
              title={t("goal.action.start")}
              // The composer-text gate is about OFFERING the button, so it stops
              // applying once the panel is open — `/goal` clears the composer on
              // its way in (every slash command does), which would otherwise
              // disable the trigger underneath its own open panel and leave the
              // dismissal with nowhere to return focus to.
              disabled={!runtimeAvailable || busy || (!open && !composerText.trim())}
              active={open}
            />
          }
        />
        <Popover.Content side="top" align="end" sideOffset={6} className="w-[360px] p-3.5">
          <form
            className="flex flex-col gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
          >
            <div>
              <h3 className="text-ui-md font-semibold text-fg">{t("goal.start.title")}</h3>
              <p className="mt-0.5 text-ui-sm leading-body text-fg-muted">
                {t("goal.start.description")}
              </p>
            </div>
            <label className="flex flex-col gap-1.5 text-ui-sm font-medium text-fg-soft">
              {t("goal.start.objective")}
              <TextArea
                value={draft.objective}
                rows={3}
                disabled={busy}
                invalid={invalid === "objective"}
                onChange={(event) => update("objective", event.target.value)}
              />
            </label>
            <fieldset className="grid grid-cols-3 gap-2" disabled={busy}>
              <legend className="mb-1.5 text-ui-sm text-fg-muted">
                {t("goal.start.optionalLimits")}
              </legend>
              <GoalLimitField
                label={t("goal.start.maxRuns")}
                value={draft.maxRuns}
                invalid={invalid === "maxRuns"}
                step="1"
                onChange={(value) => update("maxRuns", value)}
              />
              <GoalLimitField
                label={t("goal.start.maxCost")}
                value={draft.maxCostUsd}
                invalid={invalid === "maxCostUsd"}
                step="any"
                onChange={(value) => update("maxCostUsd", value)}
              />
              <GoalLimitField
                label={t("goal.start.maxSteps")}
                value={draft.maxSteps}
                invalid={invalid === "maxSteps"}
                step="1"
                onChange={(value) => update("maxSteps", value)}
              />
            </fieldset>
            <p className="-mt-1 text-ui-xs text-fg-faint">{t("goal.start.uncapped")}</p>
            <div className="flex justify-end gap-2">
              <PillButton type="button" size="sm" disabled={busy} onClick={() => changeOpen(false)}>
                {t("common.cancel")}
              </PillButton>
              <PillButton
                type="submit"
                size="sm"
                variant="solid"
                disabled={busy || !runtimeAvailable}
              >
                {busy ? t("goal.start.running") : t("goal.action.start")}
              </PillButton>
            </div>
          </form>
        </Popover.Content>
      </Popover.Root>
      {existing && (
        <ConfirmDialog
          open={replacing}
          onOpenChange={setReplacing}
          title={t("goal.replace.title")}
          body={t("goal.replace.body", { objective: existing.objective })}
          confirmLabel={t("goal.replace.confirm")}
          cancelLabel={t("common.cancel")}
          destructive
          onConfirm={() => {
            setReplacing(false);
            void start();
          }}
        />
      )}
    </>
  );
}

function GoalLimitField({
  label,
  value,
  invalid,
  step,
  onChange,
}: {
  label: string;
  value: string;
  invalid: boolean;
  step: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="flex flex-col gap-1 text-ui-xs text-fg-muted">
      {label}
      <TextField
        type="number"
        min="0"
        step={step}
        size="sm"
        value={value}
        invalid={invalid}
        aria-invalid={invalid}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}
