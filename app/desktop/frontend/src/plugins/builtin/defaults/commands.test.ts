import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { executeCommand } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { defaultCommands } from "./commands";

const mocks = vi.hoisted(() => ({
  createSession: vi.fn(),
  focusComposer: vi.fn(),
  runtimeAvailable: false,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  closeActiveAgentSession: vi.fn(),
  createSession: mocks.createSession,
}));

vi.mock("@/plugins/builtin/chat/composer/public/focus", () => ({
  focusComposer: mocks.focusComposer,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => mocks.runtimeAvailable,
}));

beforeEach(() => {
  mocks.createSession.mockReset().mockResolvedValue("session-new");
  mocks.focusComposer.mockReset();
  mocks.runtimeAvailable = false;
});

afterEach(async () => resetKernelForTest());

describe("default commands", () => {
  it("does not issue the new-Session command while Runtime commands are unavailable", async () => {
    await loadPluginsForTest(defaultCommands);

    await executeCommand("chat.new");

    expect(mocks.createSession).not.toHaveBeenCalled();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });
});
