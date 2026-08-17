import { afterEach, describe, expect, it, vi } from "vitest";
import type { ApprovalRuleSummary } from "@/plugins/builtin/agent/public/approvalPolicy";
import { forgetApprovalRules } from "./approvalConfig";

const { forgetRules } = vi.hoisted(() => ({ forgetRules: vi.fn() }));

vi.mock("@/plugins/builtin/agent/public/approvalPolicy", () => ({
  APPROVAL_MODES: [],
  agentCommandWasRetired: vi.fn(),
  forgetRule: vi.fn(),
  forgetRules,
  setApprovalMode: vi.fn(),
  useApprovalMode: vi.fn(),
  useApprovalRules: vi.fn(),
}));

afterEach(() => {
  forgetRules.mockReset();
});

describe("approval configuration", () => {
  it("delegates clear-all as one owner-scoped batch", async () => {
    forgetRules.mockResolvedValue(undefined);

    await expect(forgetApprovalRules([rule("rule-1"), rule("rule-2")])).resolves.toBeUndefined();

    expect(forgetRules).toHaveBeenCalledOnce();
    expect(forgetRules).toHaveBeenCalledWith(["rule-1", "rule-2"]);
  });
});

function rule(id: string): ApprovalRuleSummary {
  return {
    id,
    scope: "global",
    tool: "shell",
    decision: "allow",
  };
}
