import { describe, expect, it, vi } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { copyToolArgsAction, hasCopyableToolArgs } from "./toolActions";

const tool = (args: string): ToolCall => ({
  id: "tool-1",
  name: "shell",
  fn: "shell",
  args,
  status: "ok",
});

describe("hasCopyableToolArgs", () => {
  it("requires non-whitespace args", () => {
    expect(hasCopyableToolArgs(tool("pnpm test"))).toBe(true);
    expect(hasCopyableToolArgs(tool("  \n\t  "))).toBe(false);
  });
});

describe("copyToolArgsAction", () => {
  it("describes the built-in copy action", () => {
    expect(copyToolArgsAction({ title: "Copy", copyText: () => undefined })).toMatchObject({
      id: "copy-args",
      icon: "copy",
      title: "Copy",
      order: 0,
    });
  });

  it("copies the raw tool args", async () => {
    const copyText = vi.fn();
    const action = copyToolArgsAction({ title: "Copy", copyText });

    await action.run(tool("  go test ./...  "));

    expect(copyText).toHaveBeenCalledWith("  go test ./...  ");
  });
});
