import { Icon, Popover, ProgressBar } from "@/ui";
import { cn } from "@/lib/utils";
import type { TaskReadoutStatus, TaskReadoutTask } from "../application/ports/taskReadoutPort";
import { taskProgressPercent, useTaskReadout } from "../application/taskReadout";

const STATUS_ICON: Record<TaskReadoutStatus, { name: "spark" | "check" | "x"; tone: string }> = {
  running: { name: "spark", tone: "text-fg" },
  succeeded: { name: "check", tone: "text-accent" },
  failed: { name: "x", tone: "text-negative" },
};

export function TasksPill() {
  const readout = useTaskReadout();
  if (!readout) return null;

  const { name, tone } = STATUS_ICON[readout.head.status];

  return (
    <Popover.Root>
      <Popover.Trigger
        render={
          <button
            type="button"
            aria-label={readout.label}
            title={readout.title}
            className="relative grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint transition-[background,color] hover:bg-fg/[0.08] hover:text-fg active:scale-[0.96]"
          >
            <Icon
              name={name}
              size={14}
              className={cn(tone, readout.head.status === "running" && "animate-pulse-dot")}
            />
            {readout.runningCount > 0 && (
              <span className="absolute -top-0.5 -right-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-accent px-0.5 font-mono text-[9px] font-semibold text-on-accent">
                {readout.runningCount}
              </span>
            )}
          </button>
        }
      />
      <Popover.Content side="top" align="start" sideOffset={6} className="w-[320px] rounded-lg">
        <div className="px-3 pt-2 pb-1 text-[10px] font-semibold text-fg-faint">Tasks</div>
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
          size={11}
          className={cn(tone, task.status === "running" && "animate-pulse-dot")}
        />
        <span className="flex-1 truncate text-[12.5px] font-semibold text-fg">{task.label}</span>
        {percent !== null && (
          <span className="font-mono text-[11px] text-fg-faint">{percent}%</span>
        )}
      </div>
      {task.message && (
        <div className="mt-0.5 pl-[18px] text-[11.5px] text-fg-muted">{task.message}</div>
      )}
      {task.error && (
        <div className="mt-0.5 pl-[18px] text-[11.5px] text-negative">{task.error}</div>
      )}
      {percent !== null && <ProgressBar value={percent} className="mt-1.5 ml-[18px]" />}
    </div>
  );
}
