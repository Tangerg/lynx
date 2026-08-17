import { useState } from "react";
import { IconButton, Switch, type IconName } from "@/ui";
import { rpcErrorText } from "@/lib/rpcErrors";
import {
  deleteSchedule,
  runScheduleNow,
  scheduleMutationWasRetired,
  setScheduleEnabled,
  type ScheduleConfig,
} from "../application/scheduleCommands";
import { notifyError } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { formatDateTime } from "@/lib/i18n/relativeTime";
import { cn } from "@/lib/classNames";
import { ScheduleForm } from "./ScheduleForm";

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
    <IconButton
      icon={icon}
      iconSize="sm"
      size="sm"
      quiet
      aria-label={label}
      aria-expanded={active}
      title={title}
      onClick={onClick}
      className={cn(
        tone === "accent" && "hover:text-accent",
        tone === "negative" && "hover:text-negative",
      )}
    />
  );
}

export function ScheduleRow({ schedule }: { schedule: ScheduleConfig }) {
  const t = useT();
  const [editing, setEditing] = useState(false);

  const guard = async <T,>(fn: () => Promise<T>) => {
    try {
      await fn();
    } catch (err) {
      if (scheduleMutationWasRetired(err)) return;
      notifyError(rpcErrorText(err) ?? t("schedules.error.save"));
    }
  };

  return (
    <div className={cn(!schedule.enabled && "opacity-60")}>
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3 rounded-md px-3 py-2.5 transition-colors hover:bg-hover">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="truncate text-ui-md font-medium text-fg">
              {schedule.title || t("schedules.untitled")}
            </span>
            <span className="shrink-0 rounded-sm bg-surface-2 px-1.5 py-0.5 font-mono text-ui-sm text-fg-muted">
              {schedule.cron}
            </span>
          </div>
          <div
            className="mt-0.5 truncate font-mono text-ui-md leading-body text-fg-muted"
            title={schedule.instructions}
          >
            {schedule.instructions}
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 text-ui-sm text-fg-faint">
            {schedule.enabled && schedule.nextRunAt && (
              <span>{t("schedules.next", { time: formatDateTime(schedule.nextRunAt) })}</span>
            )}
            {schedule.lastRunAt && (
              <span>{t("schedules.last", { time: formatDateTime(schedule.lastRunAt) })}</span>
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
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
    </div>
  );
}
