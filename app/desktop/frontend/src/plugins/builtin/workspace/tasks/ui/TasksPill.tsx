import { useT } from "@/lib/i18n";
import { Icon, IconButton, Popover, ProgressBar, SectionLabel } from "@/ui";
import { cn } from "@/lib/classNames";
import type { TaskReadoutStatus, TaskReadoutTask } from "../application/ports/taskReadoutPort";
import { taskProgressPercent, useTaskReadout } from "../application/taskReadout";

const STATUS_ICON: Record<TaskReadoutStatus, { name: "spark" | "check" | "x"; tone: string }> = {
  running: { name: "spark", tone: "text-fg" },
  succeeded: { name: "check", tone: "text-accent" },
  failed: { name: "x", tone: "text-negative" },
};

export function TasksPill() {
  const readout = useTaskReadout();
  const t = useT();
  if (!readout) return null;

  const { name, tone } = STATUS_ICON[readout.head.status];

  return (
    <Popover.Root>
      <Popover.Trigger
        render={
          <IconButton
            icon={name}
            size="sm"
            quiet
            badge={readout.runningCount}
            aria-label={readout.label}
            className={cn(tone, readout.head.status === "running" && "[&_svg]:animate-pulse-dot")}
          />
        }
      />
      <Popover.Content side="top" align="start" sideOffset={6} className="w-[320px] rounded-xl">
        <SectionLabel className="px-3 pb-1">{t("tasks.header")}</SectionLabel>
        <div className="max-h-[280px] overflow-y-auto">
          {readout.tasks.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}

function TaskRow({ task }: { task: TaskReadoutTask }) {
  const { name, tone } = STATUS_ICON[task.status];
  const percent = taskProgressPercent(task);

  return (
    <div className="px-3 py-2">
      <div className="flex items-center gap-2">
        <Icon
          name={name}
          size="xs"
          className={cn(tone, task.status === "running" && "animate-pulse-dot")}
        />
        <span className="flex-1 truncate text-ui-md font-semibold text-fg">{task.label}</span>
        {percent !== null && <span className="font-mono text-ui-sm text-fg-faint">{percent}%</span>}
      </div>
      {task.message && (
        <div className="mt-0.5 pl-[18px] text-ui-sm text-fg-muted">{task.message}</div>
      )}
      {task.error && <div className="mt-0.5 pl-[18px] text-ui-sm text-negative">{task.error}</div>}
      {percent !== null && (
        <ProgressBar value={percent} label={task.label} className="mt-1.5 ml-[18px]" />
      )}
    </div>
  );
}
