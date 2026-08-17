import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { WorkspaceCommandActivity } from "./toolActivity";
import { TerminalViewModel, terminalSubtext } from "./terminalViewModel";

const command = (over: Partial<WorkspaceCommandActivity>): WorkspaceCommandActivity => ({
  id: "cmd-1",
  command: "npm test",
  status: "succeeded",
  output: "",
  ...over,
});

describe("TerminalViewModel", () => {
  it("projects an empty command log", () => {
    const view = TerminalViewModel.from([]);

    expect(view.commands).toEqual([]);
    expect(view.commandCount).toBe(0);
    expect(view.latestCommandId).toBe("");
    expect(view.selectedCommandId("")).toBe("");
    expect(view.isEmpty).toBe(true);
  });

  it("owns an immutable command generation and keeps its order", () => {
    const first = command({ id: "cmd-1", output: "abc" });
    const second = command({ id: "cmd-2", output: "12345" });
    const commands = [first, second];
    const view = TerminalViewModel.from(commands);

    expect(view.commands).toEqual([first, second]);
    expect(view.commands).not.toBe(commands);
    expect(Object.isFrozen(view.commands)).toBe(true);
    expect(view.commandCount).toBe(2);
    expect(view.latestCommandId).toBe("cmd-2");
    expect(view.selectedCommandId("cmd-1")).toBe("cmd-1");
    expect(view.isEmpty).toBe(false);
  });

  it("distinguishes an equal-length authoritative replacement from its live preview", () => {
    const live = TerminalViewModel.from([command({ output: "1234567" })]);
    const settled = TerminalViewModel.from([command({ output: "1\n2\n3\n4" })]);

    expect(settled).not.toBe(live);
    expect(settled.commands).not.toEqual(live.commands);
  });

  it("falls back to the latest command when compaction removed the selected tool", () => {
    const first = command({ id: "cmd-1" });
    const latest = command({ id: "cmd-2" });

    expect(TerminalViewModel.from([first, latest]).selectedCommandId("gone")).toBe("cmd-2");
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
