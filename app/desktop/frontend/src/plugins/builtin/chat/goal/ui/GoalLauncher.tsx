import { useEffect, useRef, useState } from "react";
import { IconButton, PillButton, Popover, TextArea, TextField } from "@/ui";
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
import { startGoal } from "../application/goalCommands";
import {
  parseGoalStartDraft,
  type GoalStartDraft,
  type GoalStartDraftField,
} from "../application/goalStartDraft";
import { useGoal } from "../application/goalQueries";
import {
  runtimeCommandsAvailable,
  useRuntimeCommandsAvailable,
} from "@/plugins/builtin/runtime/public/serviceStatus";

const EMPTY_LIMITS = { maxRuns: "", maxCostUsd: "", maxSteps: "" } as const;

export function GoalLauncher() {
  const sessionId = useActiveSessionId();
  return sessionId ? <SessionGoalLauncher key={sessionId} sessionId={sessionId} /> : null;
}

function SessionGoalLauncher({ sessionId }: { sessionId: string }) {
  const t = useT();
  const composerText = useComposerText();
  const setComposerText = useSetComposerText();
  const { provider, model } = useComposerModelPreference();
  const goalsAvailable = useRuntimeCapability("goals");
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const { data } = useGoal(sessionId ? { sessionId } : undefined);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
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

  const replaceable =
    !data?.goal || data.goal.status === "paused" || data.goal.status === "blocked";
  if (!goalsAvailable || data?.available !== true || !replaceable) return null;

  const update = (field: GoalStartDraftField, value: string) => {
    setDraft((current) => ({ ...current, [field]: value }));
    if (invalid === field) setInvalid(null);
  };

  const changeOpen = (next: boolean) => {
    if (busy) return;
    setOpen(next);
    setInvalid(null);
    if (next) setDraft({ objective: composerText, ...EMPTY_LIMITS });
  };

  const submit = async () => {
    if (busy || !runtimeCommandsAvailable()) return;
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
      if (mounted.current && getActiveSessionId() === sessionId) {
        notifyError(rpcErrorText(error) ?? t("goal.error.start"));
      }
    } finally {
      if (mounted.current) setBusy(false);
    }
  };

  return (
    <Popover.Root open={open} onOpenChange={changeOpen}>
      <Popover.Trigger
        render={
          <IconButton
            icon="target"
            title={t("goal.action.start")}
            disabled={!runtimeAvailable || !composerText.trim() || busy}
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
