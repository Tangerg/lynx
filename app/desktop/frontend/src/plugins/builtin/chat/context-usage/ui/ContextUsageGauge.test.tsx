import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContextUsageGauge } from "./ContextUsageGauge";

const model = vi.hoisted(() => ({
  contextTokens: 198_000,
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useSessionContextTokens: () => model.contextTokens,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => "session-a",
  useAgentSessions: () => ({ data: [{ id: "session-a", model: "gpt-5" }] }),
}));

vi.mock("@/plugins/builtin/settings/providers/public/queries", () => ({
  useModels: () => ({ data: [{ id: "gpt-5", contextWindow: 258_000 }] }),
}));

describe("ContextUsageGauge", () => {
  afterEach(cleanup);

  it("reads the latest prompt footprint instead of cumulative Run input usage", () => {
    render(<ContextUsageGauge />);

    expect(screen.getByRole("button", { name: "Context usage: 77%" })).toBeTruthy();
  });
});
