import { useState } from "react";
import { PillButton } from "@/components/common";
import { rpcErrorText } from "@/lib/rpcErrors";
import {
  createSchedule,
  updateSchedule,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import {
  CRON_PRESETS,
  type ScheduleDraft,
  canSaveScheduleDraft,
  initialScheduleDraft,
  scheduleInputFromDraft,
} from "../application/scheduleDraft";

const INPUT_CLASS =
  "w-full rounded-md border border-line-soft bg-surface px-2.5 py-1.5 text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent";

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
        await updateSchedule({ ...input, id: schedule.id, enabled: schedule.enabled });
      } else {
        await createSchedule(input);
      }
      onDone();
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("schedules.error.save"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-2.5 rounded-lg bg-surface-2 p-3 shadow-[var(--shadow-surface)]">
      <input
        value={draft.title}
        onChange={(event) => updateDraft("title", event.target.value)}
        placeholder={t("schedules.form.title")}
        aria-label={t("schedules.form.title")}
        className={INPUT_CLASS}
      />
      <textarea
        value={draft.prompt}
        onChange={(event) => updateDraft("prompt", event.target.value)}
        rows={4}
        placeholder={t("schedules.form.prompt")}
        aria-label={t("schedules.form.prompt")}
        className={cn(INPUT_CLASS, "resize-y leading-[1.5]")}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        {CRON_PRESETS.map((preset) => (
          <button
            key={preset.cron}
            type="button"
            onClick={() => updateDraft("cron", preset.cron)}
            className={cn(
              "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
              draft.cron === preset.cron
                ? "border-accent/40 bg-accent/12 text-accent"
                : "border-line-soft text-fg-muted hover:text-fg",
            )}
          >
            {t(preset.key)}
          </button>
        ))}
      </div>
      <input
        value={draft.cron}
        onChange={(event) => updateDraft("cron", event.target.value)}
        spellCheck={false}
        placeholder="0 9 * * 1-5"
        aria-label={t("schedules.form.cron")}
        className={cn(INPUT_CLASS, "font-mono")}
      />
      <input
        value={draft.cwd}
        onChange={(event) => updateDraft("cwd", event.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cwd")}
        aria-label={t("schedules.form.cwd")}
        className={cn(INPUT_CLASS, "font-mono")}
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
    </div>
  );
}
