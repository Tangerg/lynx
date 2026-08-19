import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContextUsageGauge } from "./ContextUsageGauge";

const model = vi.hoisted(() => ({
  material: {
    contextTokens: 198_000,
    progress: { contextTokens: 198_000 },
    metrics: {
      usage: { inputTokens: 900_000, outputTokens: 1_000, cacheReadTokens: 0 },
    },
  },
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useCurrentRootMaterial: () => model.material,
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

    expect(screen.getByRole("img", { name: "Context usage: 77%" })).toBeTruthy();
  });
});
