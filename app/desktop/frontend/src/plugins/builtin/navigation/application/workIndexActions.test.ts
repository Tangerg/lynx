import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { configureWorkingDirectoryPicker } from "./ports/workingDirectoryPicker";
import { useWorkIndexActions } from "./workIndexActions";

const mocks = vi.hoisted(() => ({
  activeWorkspace: { status: "ready", cwd: undefined } as
    { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string },
  choose: vi.fn(),
  create: vi.fn(),
  focusComposer: vi.fn(),
  notifyError: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  selectAgentSession: vi.fn(),
  useActiveSessionWorkspace: () => mocks.activeWorkspace,
  useCreateSession: () => mocks.create,
  useDeleteSession: () => vi.fn(),
  useForkSession: () => vi.fn(),
  useRenameSession: () => vi.fn(),
  useToggleFavorite: () => vi.fn(),
}));

vi.mock("@/plugins/builtin/chat/composer/public/focus", () => ({
  focusComposer: mocks.focusComposer,
}));

vi.mock("@/plugins/sdk", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/sdk")>()),
  notifyError: mocks.notifyError,
}));

let disposePicker = () => {};

beforeEach(() => {
  mocks.activeWorkspace = { status: "ready", cwd: undefined };
  mocks.choose.mockReset();
  mocks.create.mockReset().mockResolvedValue("session-new");
  mocks.focusComposer.mockReset();
  mocks.notifyError.mockReset();
  disposePicker = configureWorkingDirectoryPicker({ choose: mocks.choose });
});

afterEach(() => disposePicker());

describe("useWorkIndexActions directory selection", () => {
  it("starts the global new-session action in the exact active project", async () => {
    mocks.activeWorkspace = { status: "ready", cwd: "/tmp/current-project" };
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith({ cwd: "/tmp/current-project" }));
    expect(mocks.focusComposer).toHaveBeenCalledOnce();
  });

  it("does not invent a default project while the active Session is resolving", () => {
    mocks.activeWorkspace = { status: "resolving", sessionId: "session-current" };
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.createSession());

    expect(result.current.canCreateSession).toBe(false);
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("creates a session in the selected directory and focuses its composer", async () => {
    mocks.choose.mockResolvedValue("/tmp/project");
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.chooseSessionFolder());

    await waitFor(() => expect(mocks.create).toHaveBeenCalledWith({ cwd: "/tmp/project" }));
    expect(mocks.focusComposer).toHaveBeenCalledOnce();
  });

  it("treats cancellation as no mutation", async () => {
    mocks.choose.mockResolvedValue(null);
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.chooseSessionFolder());

    await waitFor(() => expect(mocks.choose).toHaveBeenCalledOnce());
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.focusComposer).not.toHaveBeenCalled();
  });

  it("coalesces repeated clicks into one picker and one session mutation", async () => {
    let release!: (cwd: string) => void;
    mocks.choose.mockImplementation(() => new Promise<string>((resolve) => (release = resolve)));
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => {
      result.current.chooseSessionFolder();
      result.current.chooseSessionFolder();
    });
    expect(mocks.choose).toHaveBeenCalledOnce();

    release("/tmp/project");
    await waitFor(() => expect(mocks.create).toHaveBeenCalledOnce());
    expect(mocks.create).toHaveBeenCalledWith({ cwd: "/tmp/project" });
  });

  it("reports native chooser failures without creating a default-cwd session", async () => {
    mocks.choose.mockRejectedValue(new Error("dialog unavailable"));
    const { result } = renderHook(() => useWorkIndexActions());

    act(() => result.current.chooseSessionFolder());

    await waitFor(() => expect(mocks.notifyError).toHaveBeenCalledOnce());
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.notifyError).toHaveBeenCalledWith(
      "Couldn't open the folder chooser.",
      expect.objectContaining({ description: "dialog unavailable", source: "session" }),
    );
  });
});
