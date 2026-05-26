// Status-bar pill that surfaces tasks registered via `host.tasks.start`.
// Hidden when no task exists; expands to a popover listing each entry
// when the user clicks.

import type { TaskEntry, TaskStatus } from "@/state/tasksStore";
import * as Popover from "@radix-ui/react-popover";
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
      {pct !== null && (
        <div className="mt-1.5 ml-[18px] h-1 rounded-full bg-surface-3 overflow-hidden">
          <div
            className={cn(
              "h-full transition-[width] duration-150",
              task.status === "failed" ? "bg-negative" : "bg-accent",
            )}
            style={{ width: `${pct}%` }}
          />
        </div>
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
  const head = running[0] ?? list[list.length - 1];
  const { name, tone } = STATUS_ICON[head.status];
  const label = running.length > 1 ? `${head.label} +${running.length - 1}` : head.label;

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="sb-item sb-btn inline-flex items-center gap-1.5 whitespace-nowrap"
          title={running.length > 0 ? `${running.length} running task(s)` : "Recent tasks"}
        >
          <Icon
            name={name}
            size={11}
            className={cn(tone, head.status === "running" && "animate-pulse-dot")}
          />
          <span className="max-w-[180px] truncate text-[11.5px] text-fg-muted">{label}</span>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="top"
          align="end"
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
    host.layout.register("app.statusbar", {
      id: "tasks",
      order: 210,
      component: TasksPill,
    });
  },
});
