import type { CommandSpec } from "@/plugins/sdk";
import { describe, expect, it } from "vitest";
import { visibleCommands } from "./commandVisibility";

const command = (id: string, when?: string): CommandSpec => ({
  id,
  label: id,
  when,
  run: () => undefined,
});

describe("visibleCommands", () => {
  it("keeps commands without a when clause", () => {
    expect(visibleCommands([command("always")], {})).toHaveLength(1);
  });

  it("applies when clauses against the current context", () => {
    const commands = [
      command("settings", 'mainView == "settings"'),
      command("diff", 'mainView == "diff"'),
      command("always"),
    ];

    expect(visibleCommands(commands, { mainView: "settings" }).map((item) => item.id)).toEqual([
      "settings",
      "always",
    ]);
  });
});
