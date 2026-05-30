// Sidebar-footer indicator that surfaces tasks registered via
// `host.tasks.start`. Hidden when no task exists; expands to a popover
// listing each entry when the user clicks.

import type { TaskEntry, TaskStatus } from "@/state/tasksStore";
import * as Popover from "@radix-ui/react-popover";
import * as Progress from "@radix-ui/react-progress";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { useTasksStore } from "@/state/tasksStore";

// Glyph + tint by task status. `running` uses a generic spark + a pulse
// animation (applied at render time) to fake a lightweight spinner.
const STATUS_ICON: Record<TaskStatus, { name: "spark" | "check" | "x"; tone: string }> = {
  running: { name: "spark", tone: "text-fg" },
  succeeded: { name: "check", tone: "text-accent" },
  failed: { name: "x", tone: "text-negative" },
};

function TaskRow({ task }: { task: TaskEntry }) {
  const { name, tone } = STATUS_ICON[task.status];
  const pct =
    task.progress !== null && task.status !== "failed"
      ? Math.round(Math.max(0, Math.min(1, task.progress)) * 100)
      : null;
  return (
    <div className="px-3 py-2">
      <div className="flex items-center gap-2">
        <Icon
          name={name}
          size={11}
          className={cn(tone, task.status === "running" && "animate-pulse-dot")}
        />
        <span className="flex-1 truncate text-[12.5px] font-semibold text-fg">{task.label}</span>
        {pct !== null && (
          <span className="font-mono text-[11px] tabular-nums text-fg-faint">{pct}%</span>
        )}
      </div>
      {task.message && (
        <div className="mt-0.5 pl-[18px] text-[11.5px] text-fg-muted">{task.message}</div>
      )}
      {task.error && (
        <div className="mt-0.5 pl-[18px] text-[11.5px] text-negative">{task.error}</div>
      )}
      {/* Radix Progress → role=progressbar + aria-valuenow/max for SR.
          Only rendered when `pct` is set, which excludes failed tasks,
          so the fill is always the accent tone. */}
      {pct !== null && (
        <Progress.Root
          value={pct}
          className="mt-1.5 ml-[18px] h-1 overflow-hidden rounded-full bg-surface-3"
        >
          <Progress.Indicator
            className="h-full bg-accent transition-[width] duration-150"
            style={{ width: `${pct}%` }}
          />
        </Progress.Root>
      )}
    </div>
  );
}

function TasksPill() {
  // Subscribe to the map identity so we re-render on add/remove. The
  // values themselves are tracked inside TaskRow.
  const tasks = useTasksStore((s) => s.tasks);
  if (tasks.size === 0) return null;

  const list = Array.from(tasks.values()).sort((a, b) => a.startedAt - b.startedAt);
  const running = list.filter((t) => t.status === "running");
  // tasks.size > 0 above guarantees list non-empty → head exists.
  const head = running[0] ?? list.at(-1)!;
  const { name, tone } = STATUS_ICON[head.status];
  const label = running.length > 1 ? `${head.label} +${running.length - 1}` : head.label;

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label={label}
          title={running.length > 0 ? `${running.length} running task(s)` : "Recent tasks"}
          className="relative grid h-6.5 w-6.5 place-items-center rounded-md border-0 bg-transparent text-fg-faint cursor-pointer transition-[background,color] hover:bg-surface-2 hover:text-fg light:hover:bg-surface-3 active:scale-[0.92]"
        >
          <Icon
            name={name}
            size={14}
            className={cn(tone, head.status === "running" && "animate-pulse-dot")}
          />
          {running.length > 0 && (
            <span className="absolute -right-0.5 -top-0.5 grid h-3.5 min-w-3.5 place-items-center rounded-full bg-accent px-0.5 font-mono text-[9px] font-semibold tabular-nums text-on-accent">
              {running.length}
            </span>
          )}
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="top"
          align="start"
          sideOffset={6}
          className="z-50 w-[320px] overflow-hidden rounded-lg border border-line bg-surface shadow-lg"
        >
          <div className="px-3 pt-2 pb-1 text-[10px] font-semibold tracking-wider text-fg-faint uppercase">
            Tasks
          </div>
          <div className="max-h-[280px] overflow-y-auto">
            {list.map((task) => (
              <TaskRow key={task.id} task={task} />
            ))}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

export const tasksPill = definePlugin({
  name: "lyra.builtin.tasks",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.footer.status", {
      id: "tasks",
      order: 0,
      component: TasksPill,
    });
  },
});
