import type { Schedule } from "@/rpc";
import { useState } from "react";
import { Icon, type IconName, Switch } from "@/components/common";
import { rpcErrorText } from "@/lib/agent/errorCopy";
import { deleteSchedule, runScheduleNow, setScheduleEnabled } from "@/lib/agent/scheduleConfig";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { ScheduleForm } from "./ScheduleForm";

function formatScheduleTime(iso?: string): string {
  if (!iso) return "";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleString();
}

function ScheduleActionButton({
  icon,
  label,
  title,
  active,
  tone,
  onClick,
}: {
  icon: IconName;
  label: string;
  title?: string;
  active?: boolean;
  tone?: "accent" | "negative";
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-expanded={active}
      title={title}
      onClick={onClick}
      className={cn(
        "grid h-7 w-7 place-items-center rounded-md text-fg-faint transition-colors hover:bg-surface-2",
        tone === "accent" && "hover:text-accent",
        tone === "negative" && "hover:text-negative",
        !tone && "hover:text-fg",
      )}
    >
      <Icon name={icon} size={13} />
    </button>
  );
}

export function ScheduleRow({ schedule, defaultCwd }: { schedule: Schedule; defaultCwd?: string }) {
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
        !schedule.enabled && "opacity-60",
      )}
    >
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate text-[14px] font-semibold text-fg">
              {schedule.title || t("schedules.untitled")}
            </span>
            <span className="shrink-0 rounded-xs bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] text-fg-muted">
              {schedule.cron}
            </span>
          </div>
          <div
            className="mt-0.5 truncate text-[12px] leading-[1.45] text-fg-muted"
            title={schedule.prompt}
          >
            {schedule.prompt}
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 text-[11px] text-fg-faint">
            {schedule.enabled && schedule.nextRunAt && (
              <span>{t("schedules.next", { time: formatScheduleTime(schedule.nextRunAt) })}</span>
            )}
            {schedule.lastRunAt && (
              <span>{t("schedules.last", { time: formatScheduleTime(schedule.lastRunAt) })}</span>
            )}
          </div>
        </div>
        <div className="flex items-center gap-1.5">
          <Switch
            checked={schedule.enabled}
            onCheckedChange={(value) => void guard(() => setScheduleEnabled(schedule, value))}
            ariaLabel={t("schedules.enable.aria")}
          />
          <ScheduleActionButton
            icon="play"
            label={t("schedules.runNow")}
            title={t("schedules.runNow")}
            tone="accent"
            onClick={() => void guard(() => runScheduleNow(schedule.id))}
          />
          <ScheduleActionButton
            icon="edit"
            label={t("schedules.edit")}
            active={editing}
            onClick={() => setEditing((value) => !value)}
          />
          <ScheduleActionButton
            icon="trash"
            label={t("schedules.delete")}
            title={t("schedules.delete")}
            tone="negative"
            onClick={() => void guard(() => deleteSchedule(schedule.id))}
          />
        </div>
      </div>

      {editing && (
        <div className="mt-2.5">
          <ScheduleForm
            schedule={schedule}
            defaultCwd={defaultCwd}
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
    </div>
  );
}
