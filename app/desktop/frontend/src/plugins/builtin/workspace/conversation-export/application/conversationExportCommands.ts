import type { CommandSpec } from "@/plugins/sdk";

export type CommandRun = CommandSpec["run"];

export interface ConversationExportCommandHandlers {
  exportMarkdown: CommandRun;
  exportJson: CommandRun;
  importJson: CommandRun;
}

export function conversationExportCommands(
  handlers: ConversationExportCommandHandlers,
): CommandSpec[] {
  return [
    {
      id: "chat.export.markdown",
      label: "convExport.markdown",
      run: handlers.exportMarkdown,
    },
    {
      id: "chat.export.json",
      label: "convExport.json",
      run: handlers.exportJson,
    },
    {
      id: "chat.import.json",
      label: "convExport.import",
      run: handlers.importJson,
    },
  ];
}
