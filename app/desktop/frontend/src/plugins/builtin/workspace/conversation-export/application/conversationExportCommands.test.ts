import { describe, expect, it, vi } from "vitest";
import { conversationExportCommands } from "./conversationExportCommands";

describe("conversationExportCommands", () => {
  it("projects translated labels and handlers into stable command specs", () => {
    const handlers = {
      exportMarkdown: vi.fn(),
      exportJson: vi.fn(),
      importJson: vi.fn(),
    };

    expect(conversationExportCommands((key) => `t:${key}`, handlers)).toEqual([
      {
        id: "chat.export.markdown",
        label: "t:convExport.markdown",
        icon: "filetext",
        group: "Chat",
        keywords: ["save", "download", "export"],
        run: handlers.exportMarkdown,
      },
      {
        id: "chat.export.json",
        label: "t:convExport.json",
        icon: "code",
        group: "Chat",
        keywords: ["save", "download", "export", "archive"],
        run: handlers.exportJson,
      },
      {
        id: "chat.import.json",
        label: "t:convExport.import",
        icon: "history",
        group: "Chat",
        keywords: ["restore", "load", "import"],
        run: handlers.importJson,
      },
    ]);
  });
});
