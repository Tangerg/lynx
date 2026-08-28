import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContextUsageGauge } from "./ContextUsageGauge";

const model = vi.hoisted(() => ({
  contextTokens: 198_000,
  modelSelection: { provider: "provider-b", model: "shared-model", reasoningEffort: "high" },
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useCurrentRootMaterial: () => model,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => "session-a",
  useAgentSessions: () => ({
    data: [{ id: "session-a", provider: "provider-a", model: "shared-model" }],
  }),
}));

vi.mock("@/plugins/builtin/settings/providers/public/queries", () => ({
  useModels: () => ({
    data: [
      { provider: "provider-a", id: "shared-model", contextWindow: 100_000 },
      { provider: "provider-b", id: "shared-model", contextWindow: 258_000 },
    ],
  }),
}));

describe("ContextUsageGauge", () => {
  afterEach(cleanup);

  it("reads the active Run's exact provider/model window instead of the editable Session default", () => {
    render(<ContextUsageGauge />);

    expect(screen.getByRole("button", { name: "Context usage: 77%" })).toBeTruthy();
  });
});
