import { useState } from "react";
import { FIELD_CLASSES, PillButton } from "@/ui";
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

const FIELD = cn(FIELD_CLASSES, "w-full px-2.5 py-1.5 text-fg placeholder:text-fg-faint");

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
      notifyError(rpcErrorText(err) ?? t("schedules.error.save"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-3 rounded-[14px] bg-surface p-4">
      <input
        value={draft.title}
        onChange={(event) => updateDraft("title", event.target.value)}
        placeholder={t("schedules.form.title")}
        aria-label={t("schedules.form.title")}
        className={cn(FIELD, "font-sans")}
      />
      <textarea
        value={draft.prompt}
        onChange={(event) => updateDraft("prompt", event.target.value)}
        rows={4}
        placeholder={t("schedules.form.prompt")}
        aria-label={t("schedules.form.prompt")}
        className={cn(FIELD, "resize-y font-sans leading-[1.5]")}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        {CRON_PRESETS.map((preset) => (
          <button
            key={preset.cron}
            type="button"
            onClick={() => updateDraft("cron", preset.cron)}
            className={cn(
              "rounded-pill px-2.5 py-1 text-[11px] font-medium transition-colors",
              draft.cron === preset.cron ? "bg-fg/[0.075] text-fg" : "text-fg hover:bg-fg/[0.04]",
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
        className={FIELD}
      />
      <input
        value={draft.cwd}
        onChange={(event) => updateDraft("cwd", event.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cwd")}
        aria-label={t("schedules.form.cwd")}
        className={FIELD}
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
