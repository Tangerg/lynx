import type { CommandSpec, WhenContext } from "@/plugins/sdk";
import { evalWhen } from "@/plugins/sdk";

export function visibleCommands(commands: CommandSpec[], whenContext: WhenContext): CommandSpec[] {
  return commands.filter((command) => !command.when || evalWhen(command.when, whenContext));
}
