import type { WorkspaceCommandActivity } from "./toolActivity";

export interface TerminalViewModel {
  commands: WorkspaceCommandActivity[];
  commandCount: number;
  tailSignature: number;
  isEmpty: boolean;
}

export function terminalViewModel(
  commands: readonly WorkspaceCommandActivity[],
): TerminalViewModel {
  let tailSignature = commands.length;
  for (const command of commands) {
    tailSignature += command.output.length;
  }

  return {
    commands: Array.from(commands),
    commandCount: commands.length,
    tailSignature,
    isEmpty: commands.length === 0,
  };
}

export function terminalSubtext({
  commandCount,
}: Pick<TerminalViewModel, "commandCount">): string | undefined {
  if (commandCount === 0) {
    return undefined;
  }
  return `${commandCount} commands`;
}
