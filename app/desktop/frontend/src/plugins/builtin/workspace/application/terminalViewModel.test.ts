import { describe, expect, it } from "vitest";
import type { WorkspaceCommandActivity } from "./toolActivity";
import { terminalSubtext, terminalViewModel } from "./terminalViewModel";

const command = (over: Partial<WorkspaceCommandActivity>): WorkspaceCommandActivity => ({
  id: "cmd-1",
  command: "npm test",
  status: "succeeded",
  output: "",
  outputTruncated: false,
  ...over,
});

describe("terminalViewModel", () => {
  it("projects an empty command log", () => {
    expect(terminalViewModel([])).toEqual({
      commands: [],
      commandCount: 0,
      tailSignature: 0,
      isEmpty: true,
    });
  });

  it("keeps command order and computes a tail signature from count and output length", () => {
    const first = command({ id: "cmd-1", output: "abc" });
    const second = command({ id: "cmd-2", output: "12345" });

    expect(terminalViewModel([first, second])).toEqual({
      commands: [first, second],
      commandCount: 2,
      tailSignature: 10,
      isEmpty: false,
    });
  });
});

describe("terminalSubtext", () => {
  it("omits header subtext when there are no commands", () => {
    expect(terminalSubtext({ commandCount: 0 })).toBeUndefined();
  });

  it("builds command count header text", () => {
    expect(terminalSubtext({ commandCount: 1 })).toBe("1 commands");
  });
});
