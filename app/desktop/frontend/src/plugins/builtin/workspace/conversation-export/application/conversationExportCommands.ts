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
      icon: "filetext",
      group: "command.group.chat",
      keywords: ["save", "download", "export"],
      run: handlers.exportMarkdown,
    },
    {
      id: "chat.export.json",
      label: "convExport.json",
      icon: "code",
      group: "command.group.chat",
      keywords: ["save", "download", "export", "archive"],
      run: handlers.exportJson,
    },
    {
      id: "chat.import.json",
      label: "convExport.import",
      icon: "history",
      group: "command.group.chat",
      keywords: ["restore", "load", "import"],
      run: handlers.importJson,
    },
  ];
}
