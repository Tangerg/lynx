import type { CommandSpec } from "@/plugins/sdk";

export type Translate = (key: string) => string;
export type CommandRun = CommandSpec["run"];

export interface ConversationExportCommandHandlers {
  exportMarkdown: CommandRun;
  exportJson: CommandRun;
  importJson: CommandRun;
}

export function conversationExportCommands(
  t: Translate,
  handlers: ConversationExportCommandHandlers,
): CommandSpec[] {
  return [
    {
      id: "chat.export.markdown",
      label: t("convExport.markdown"),
      icon: "filetext",
      group: "Chat",
      keywords: ["save", "download", "export"],
      run: handlers.exportMarkdown,
    },
    {
      id: "chat.export.json",
      label: t("convExport.json"),
      icon: "code",
      group: "Chat",
      keywords: ["save", "download", "export", "archive"],
      run: handlers.exportJson,
    },
    {
      id: "chat.import.json",
      label: t("convExport.import"),
      icon: "history",
      group: "Chat",
      keywords: ["restore", "load", "import"],
      run: handlers.importJson,
    },
  ];
}
