import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { WorkspaceCommandActivity } from "./toolActivity";
import { terminalSubtext, terminalViewModel } from "./terminalViewModel";

const command = (over: Partial<WorkspaceCommandActivity>): WorkspaceCommandActivity => ({
  id: "cmd-1",
  command: "npm test",
  status: "succeeded",
  output: "",
  ...over,
});

describe("terminalViewModel", () => {
  it("projects an empty command log", () => {
    expect(terminalViewModel([])).toEqual({
      commands: [],
      commandCount: 0,
      tailSignature: 0,
      selectedCommandId: "",
      isEmpty: true,
    });
  });

  it("keeps command order and computes a tail signature from count and output length", () => {
    const first = command({ id: "cmd-1", output: "abc" });
    const second = command({ id: "cmd-2", output: "12345" });

    expect(terminalViewModel([first, second], "cmd-1")).toEqual({
      commands: [first, second],
      commandCount: 2,
      tailSignature: 10,
      selectedCommandId: "cmd-1",
      isEmpty: false,
    });
  });

  it("falls back to the latest command when compaction removed the selected tool", () => {
    const first = command({ id: "cmd-1" });
    const latest = command({ id: "cmd-2" });

    expect(terminalViewModel([first, latest], "gone").selectedCommandId).toBe("cmd-2");
  });
});

describe("terminalSubtext", () => {
  it("omits header subtext when there are no commands", () => {
    expect(terminalSubtext(t, { commandCount: 0 })).toBeUndefined();
  });

  it("builds command count header text", () => {
    expect(terminalSubtext(t, { commandCount: 1 })).toBe("1 commands");
  });
});
