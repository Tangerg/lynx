import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { executeCommand } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { defaultCommands } from "./commands";

const mocks = vi.hoisted(() => ({
  activeSessionId: "",
  createSession: vi.fn(),
  focusComposer: vi.fn(),
  runtimeAvailable: false,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  closeActiveAgentSession: vi.fn(),
  createSession: mocks.createSession,
  getActiveSessionId: () => mocks.activeSessionId,
}));

vi.mock("@/plugins/builtin/chat/composer/public/focus", () => ({
  focusComposer: mocks.focusComposer,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => mocks.runtimeAvailable,
}));

beforeEach(() => {
  mocks.activeSessionId = "";
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

  it("focuses the project-selection destination without allocating a default-workspace Session", async () => {
    mocks.runtimeAvailable = true;
    await loadPluginsForTest(defaultCommands);

    await executeCommand("chat.new");

    expect(mocks.createSession).not.toHaveBeenCalled();
    expect(mocks.focusComposer).toHaveBeenCalledOnce();
  });

  it("delegates New to the active Session's exact-workspace owner", async () => {
    mocks.runtimeAvailable = true;
    mocks.activeSessionId = "session-current";
    await loadPluginsForTest(defaultCommands);

    await executeCommand("chat.new");

    expect(mocks.createSession).toHaveBeenCalledOnce();
    expect(mocks.focusComposer).toHaveBeenCalledOnce();
  });
});
