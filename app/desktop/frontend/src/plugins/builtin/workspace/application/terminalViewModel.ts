import type { Translate } from "@/lib/i18n";
import type { WorkspaceCommandActivity } from "./toolActivity";

export class TerminalViewModel {
  readonly commands: readonly WorkspaceCommandActivity[];
  readonly commandCount: number;
  readonly latestCommandId: string;
  readonly isEmpty: boolean;

  private constructor(commands: readonly WorkspaceCommandActivity[]) {
    this.commands = Object.freeze(commands.map((command) => Object.freeze({ ...command })));
    this.commandCount = commands.length;
    this.latestCommandId = commands.at(-1)?.id ?? "";
    this.isEmpty = commands.length === 0;
  }

  static from(commands: readonly WorkspaceCommandActivity[]): TerminalViewModel {
    return new TerminalViewModel(commands);
  }

  selectedCommandId(selectedToolId: string): string {
    return this.commands.some((command) => command.id === selectedToolId)
      ? selectedToolId
      : this.latestCommandId;
  }
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
