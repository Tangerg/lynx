// Built-in plugin: "Schedules" settings pane. Manages the cron-triggered
// headless runs (schedules.*) — create / edit / enable / run-now / delete. The
// runtime's scheduler worker fires each schedule's saved prompt as a fresh
// headless run while `lyra serve` is up; those runs show up as new sessions in
// the sidebar (the schedules.fired event refreshes it).
//
// A schedule stores the final prompt text; "Fill from a recipe" is left to the
// user pasting one — the pane stays decoupled from recipes, like the runtime.

import type { Schedule } from "@/rpc";
import { useState } from "react";
import { DataView, EmptyState, Icon, PillButton, Switch } from "@/components/common";
import { isUnsupportedMethod, rpcErrorText } from "@/lib/agent/errorCopy";
import {
  createSchedule,
  deleteSchedule,
  runScheduleNow,
  setScheduleEnabled,
  updateSchedule,
} from "@/lib/agent/scheduleConfig";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useSchedules } from "@/lib/data/queries";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";

// Common cron shapes, surfaced as one-tap chips so the bare expression stays
// optional. Values are 5-field standard cron ("min hour dom month dow").
const CRON_PRESETS: Array<{ key: string; cron: string }> = [
  { key: "schedules.preset.hourly", cron: "0 * * * *" },
  { key: "schedules.preset.daily", cron: "0 9 * * *" },
  { key: "schedules.preset.weekdays", cron: "0 9 * * 1-5" },
  { key: "schedules.preset.weekly", cron: "0 9 * * 1" },
];

const INPUT_CLASS =
  "w-full rounded-md border border-line-soft bg-surface px-2.5 py-1.5 text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent";

function fmtTime(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString();
}

function ScheduleForm({
  schedule,
  defaultCwd,
  onDone,
  onCancel,
}: {
  schedule?: Schedule;
  defaultCwd?: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const t = useT();
  const [title, setTitle] = useState(schedule?.title ?? "");
  const [prompt, setPrompt] = useState(schedule?.prompt ?? "");
  const [cron, setCron] = useState(schedule?.cron ?? "0 9 * * 1-5");
  const [cwd, setCwd] = useState(schedule?.cwd ?? defaultCwd ?? "");
  const [busy, setBusy] = useState(false);

  const canSave = prompt.trim() !== "" && cron.trim() !== "" && !busy;

  const onSave = async () => {
    setBusy(true);
    try {
      const input = {
        title: title.trim(),
        prompt: prompt.trim(),
        cwd: cwd.trim(),
        cron: cron.trim(),
      };
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
    <div className="flex flex-col gap-2.5 rounded-lg border border-line-soft bg-surface-2 p-3">
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        placeholder={t("schedules.form.title")}
        aria-label={t("schedules.form.title")}
        className={INPUT_CLASS}
      />
      <textarea
        value={prompt}
        onChange={(e) => setPrompt(e.target.value)}
        rows={4}
        placeholder={t("schedules.form.prompt")}
        aria-label={t("schedules.form.prompt")}
        className={cn(INPUT_CLASS, "resize-y leading-[1.5]")}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        {CRON_PRESETS.map((p) => (
          <button
            key={p.cron}
            type="button"
            onClick={() => setCron(p.cron)}
            className={cn(
              "rounded-full border px-2 py-0.5 text-[11px] transition-colors",
              cron === p.cron
                ? "border-accent/40 bg-accent/12 text-accent"
                : "border-line-soft text-fg-muted hover:text-fg",
            )}
          >
            {t(p.key)}
          </button>
        ))}
      </div>
      <input
        value={cron}
        onChange={(e) => setCron(e.target.value)}
        spellCheck={false}
        placeholder="0 9 * * 1-5"
        aria-label={t("schedules.form.cron")}
        className={cn(INPUT_CLASS, "font-mono")}
      />
      <input
        value={cwd}
        onChange={(e) => setCwd(e.target.value)}
        spellCheck={false}
        placeholder={t("schedules.form.cwd")}
        aria-label={t("schedules.form.cwd")}
        className={cn(INPUT_CLASS, "font-mono")}
      />
      <div className="flex items-center gap-2">
        <PillButton variant="accent" size="sm" disabled={!canSave} onClick={() => void onSave()}>
          {busy ? t("schedules.saving") : t("schedules.save")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </PillButton>
      </div>
    </div>
  );
}

function ScheduleRow({ s, defaultCwd }: { s: Schedule; defaultCwd?: string }) {
  const t = useT();
  const [editing, setEditing] = useState(false);

  const guard = async (fn: () => Promise<void>) => {
    try {
      await fn();
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("schedules.error.save"));
    }
  };

  return (
    <div
      className={cn(
        "rounded-lg border border-line-soft bg-canvas px-3 py-2.5",
        !s.enabled && "opacity-60",
      )}
    >
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate text-[14px] font-semibold text-fg">
              {s.title || t("schedules.untitled")}
            </span>
            <span className="shrink-0 rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] text-fg-muted">
              {s.cron}
            </span>
          </div>
          <div
            className="mt-0.5 truncate text-[12px] leading-[1.45] text-fg-muted"
            title={s.prompt}
          >
            {s.prompt}
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 text-[11px] text-fg-faint">
            {s.enabled && s.nextRunAt && (
              <span>{t("schedules.next", { time: fmtTime(s.nextRunAt) })}</span>
            )}
            {s.lastRunAt && <span>{t("schedules.last", { time: fmtTime(s.lastRunAt) })}</span>}
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          <Switch
            checked={s.enabled}
            onCheckedChange={(v) => void guard(() => setScheduleEnabled(s, v))}
            ariaLabel={t("schedules.enable.aria")}
          />
          <button
            type="button"
            aria-label={t("schedules.runNow")}
            title={t("schedules.runNow")}
            onClick={() => void guard(() => runScheduleNow(s.id))}
            className="grid h-7 w-7 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2 hover:text-accent"
          >
            <Icon name="play" size={13} />
          </button>
          <button
            type="button"
            aria-label={t("schedules.edit")}
            aria-expanded={editing}
            onClick={() => setEditing((v) => !v)}
            className="grid h-7 w-7 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2 hover:text-fg"
          >
            <Icon name="edit" size={13} />
          </button>
          <button
            type="button"
            aria-label={t("schedules.delete")}
            title={t("schedules.delete")}
            onClick={() => void guard(() => deleteSchedule(s.id))}
            className="grid h-7 w-7 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2 hover:text-negative"
          >
            <Icon name="trash" size={13} />
          </button>
        </div>
      </div>

      {editing && (
        <div className="mt-2.5">
          <ScheduleForm
            schedule={s}
            defaultCwd={defaultCwd}
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
    </div>
  );
}

function SchedulesPane() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const { data, isLoading, isError, error } = useSchedules();
  const [adding, setAdding] = useState(false);

  if (isError && isUnsupportedMethod(error)) {
    return (
      <EmptyState
        icon="command"
        title={t("schedules.unavailable")}
        sub={t("schedules.unavailable.sub")}
      />
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-[13px] leading-[1.5] text-fg-muted">{t("schedules.intro")}</p>

      {adding ? (
        <ScheduleForm
          defaultCwd={cwd}
          onDone={() => setAdding(false)}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <div className="flex justify-end">
          <PillButton variant="outlined" size="sm" onClick={() => setAdding(true)}>
            <Icon name="plus" size={13} />
            {t("schedules.add")}
          </PillButton>
        </div>
      )}

      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{ icon: "command", title: t("schedules.empty"), sub: t("schedules.empty.sub") }}
      >
        {(rows) => (
          <div className="flex flex-col gap-2">
            {rows.map((s) => (
              <ScheduleRow key={s.id} s={s} defaultCwd={cwd} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.schedules-pane",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(SETTINGS_PANE, {
      id: "schedules",
      label: "settings.pane.schedules",
      group: "agent",
      icon: "command",
      // After Hooks (57) — both extend "what runs around the agent" off the
      // main chat loop; schedules are the time-triggered surface.
      order: 58,
      component: SchedulesPane,
    });
  },
});
