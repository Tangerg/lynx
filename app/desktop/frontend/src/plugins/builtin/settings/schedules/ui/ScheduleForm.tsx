import { useState } from "react";
import { PillButton, Pressable, Surface, TextArea, TextField } from "@/ui";
import { rpcErrorText } from "@/lib/rpcErrors";
import {
  createSchedule,
  scheduleMutationWasRetired,
  updateSchedule,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";
import {
  CRON_PRESETS,
  type ScheduleDraft,
  canSaveScheduleDraft,
  initialScheduleDraft,
  scheduleInputFromDraft,
} from "../application/scheduleDraft";

interface ScheduleFormProps {
  schedule?: ScheduleConfig;
  defaultCwd?: string;
  onDone: () => void;
  onCancel: () => void;
}

export function ScheduleForm({ schedule, defaultCwd, onDone, onCancel }: ScheduleFormProps) {
  const t = useT();
  const [draft, setDraft] = useState<ScheduleDraft>(() =>
    initialScheduleDraft(schedule, defaultCwd),
  );
  const [busy, setBusy] = useState(false);

  const updateDraft = <K extends keyof ScheduleDraft>(key: K, value: ScheduleDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const onSave = async () => {
    setBusy(true);
    try {
      const input = scheduleInputFromDraft(draft);
      if (schedule) {
        await updateSchedule({
          ...input,
          id: schedule.id,
          enabled: schedule.enabled,
          revision: schedule.revision,
        });
      } else {
        await createSchedule(input);
      }
      onDone();
    } catch (err) {
      if (scheduleMutationWasRetired(err)) return;
      notifyError(rpcErrorText(err) ?? t("schedules.error.save"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Surface className="flex flex-col gap-3">
      <TextField
        font="sans"
        value={draft.title}
        onChange={(event) => updateDraft("title", event.target.value)}
        placeholder={t("schedules.form.title")}
        aria-label={t("schedules.form.title")}
      />
      <TextArea
        font="sans"
        size="sm"
        value={draft.instructions}
        onChange={(event) => updateDraft("instructions", event.target.value)}
        rows={4}
        placeholder={t("schedules.form.instructions")}
        aria-label={t("schedules.form.instructions")}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        {CRON_PRESETS.map((preset) => (
          <Pressable
            key={preset.cron}
            type="button"
            onClick={() => updateDraft("cron", preset.cron)}
            className={cn(
              "rounded-pill px-2.5 py-1 text-ui-sm font-medium transition-colors",
              draft.cron === preset.cron ? "bg-selected text-fg" : "text-fg hover:bg-hover",
            )}
          >
            {t(preset.key)}
          </Pressable>
        ))}
      </div>
      <TextField
        value={draft.cron}
        onChange={(event) => updateDraft("cron", event.target.value)}
        spellCheck={false}
        placeholder="0 9 * * 1-5"
        aria-label={t("schedules.form.cron")}
      />
      <TextField
        value={draft.cwd}
        onChange={(event) => updateDraft("cwd", event.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cwd")}
        aria-label={t("schedules.form.cwd")}
      />
      <div className="flex items-center gap-2">
        <PillButton
          variant="accent"
          size="sm"
          disabled={!canSaveScheduleDraft(draft, busy)}
          onClick={() => void onSave()}
        >
          {busy ? t("schedules.saving") : t("schedules.save")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </PillButton>
      </div>
    </Surface>
  );
}
