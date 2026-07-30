import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { toolGroupModel } from "./toolGroupModel";

const tool = ({ runId = "run_1", ...overrides }: Partial<ToolCall> = {}): ToolCall => ({
  id: "tool-1",
  runId,
  name: "read",
  fn: "read",
  args: "",
  status: "ok",
  ...overrides,
});

describe("toolGroupModel", () => {
  it("follows attention when the group is not pinned", () => {
    expect(toolGroupModel(t, [tool({ status: "running" })], null)).toMatchObject({
      needsAttention: true,
      expanded: true,
      nextPinned: false,
    });

    expect(toolGroupModel(t, [tool({ status: "ok" })], null)).toMatchObject({
      needsAttention: false,
      expanded: false,
      nextPinned: true,
    });
  });

  it("lets a user pin override attention", () => {
    expect(toolGroupModel(t, [tool({ status: "running" })], false)).toMatchObject({
      needsAttention: true,
      expanded: false,
      nextPinned: true,
    });

    expect(toolGroupModel(t, [tool({ status: "ok" })], true)).toMatchObject({
      needsAttention: false,
      expanded: true,
      nextPinned: false,
    });
  });

  it("projects stable summary and count for the group header", () => {
    expect(
      toolGroupModel(
        t,
        [tool({ id: "read", name: "read" }), tool({ id: "grep", name: "grep" })],
        null,
      ),
    ).toMatchObject({
      summary: "1 read · 1 search",
      count: 2,
    });
  });
});
