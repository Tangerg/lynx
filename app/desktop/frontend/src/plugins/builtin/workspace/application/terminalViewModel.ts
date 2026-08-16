import type { Translate } from "@/lib/i18n";
import type { WorkspaceCommandActivity } from "./toolActivity";

export interface TerminalViewModel {
  commands: WorkspaceCommandActivity[];
  commandCount: number;
  tailSignature: number;
  selectedCommandId: string;
  isEmpty: boolean;
}

export function terminalViewModel(
  commands: readonly WorkspaceCommandActivity[],
  selectedToolId = "",
): TerminalViewModel {
  let tailSignature = commands.length;
  for (const command of commands) {
    tailSignature += command.output.length;
  }

  const selectedCommandId = commands.some((command) => command.id === selectedToolId)
    ? selectedToolId
    : (commands.at(-1)?.id ?? "");

  return {
    commands: Array.from(commands),
    commandCount: commands.length,
    tailSignature,
    selectedCommandId,
    isEmpty: commands.length === 0,
  };
}

export function terminalSubtext(
  t: Translate,
  { commandCount }: Pick<TerminalViewModel, "commandCount">,
): string | undefined {
  if (commandCount === 0) {
    return undefined;
  }
  return t("terminal.commands", { count: commandCount });
}
