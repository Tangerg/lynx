import { describe, expect, it, vi } from "vitest";
import { conversationExportCommands } from "./conversationExportCommands";

describe("conversationExportCommands", () => {
  it("projects catalog keys and handlers into stable command specs", () => {
    const handlers = {
      exportMarkdown: vi.fn(),
      exportJson: vi.fn(),
      importJson: vi.fn(),
    };

    expect(conversationExportCommands(handlers)).toEqual([
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
    ]);
  });
});
