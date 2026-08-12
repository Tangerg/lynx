import { beforeEach, describe, expect, it, vi } from "vitest";
import { subscribeRuntimeWorkspaceEvents } from "./runtimeWorkspaceEvents";

const { fileWatch, resolveWorkspace, subscribe } = vi.hoisted(() => ({
  fileWatch: vi.fn(() => true),
  resolveWorkspace: vi.fn(),
  subscribe: vi.fn(),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: fileWatch,
  runtimeSupportsStreamingMethod: vi.fn(() => true),
}));

vi.mock("@/main/container", () => ({
  getContainer: () => ({
    client: () => ({
      workspaces: { resolve: resolveWorkspace },
      runtimeEvents: { subscribe },
    }),
  }),
}));

const events = {
  async *[Symbol.asyncIterator]() {},
};

beforeEach(() => {
  fileWatch.mockReturnValue(true);
  resolveWorkspace.mockReset();
  subscribe.mockReset();
  subscribe.mockResolvedValue({ result: {}, events });
});

describe("runtime workspace event subscription", () => {
  it("uses the canonical available workspace as the file-watch scope", async () => {
    resolveWorkspace.mockResolvedValue({
      ref: { path: "/canonical/repo" },
      projectRoot: "/canonical/repo",
      availability: "available",
    });
    const signal = new AbortController().signal;

    await expect(
      subscribeRuntimeWorkspaceEvents({ type: "workspace", cwd: "/linked/repo" }, signal),
    ).resolves.toBe(events);

    expect(resolveWorkspace).toHaveBeenCalledWith({ path: "/linked/repo" }, signal);
    expect(subscribe).toHaveBeenCalledWith(
      expect.objectContaining({
        topics: expect.arrayContaining(["knowledge.changed", "hooks.changed"]),
        watches: [{ watchId: "active-session", workspace: { path: "/canonical/repo" } }],
      }),
      signal,
    );
  });

  it("cancels watch-root resolution with the subscription lifecycle", async () => {
    const controller = new AbortController();
    const reason = new Error("retargeted");
    resolveWorkspace.mockImplementation(
      (_ref: unknown, signal: AbortSignal) =>
        new Promise((_, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason), { once: true });
        }),
    );

    const opening = subscribeRuntimeWorkspaceEvents(
      { type: "workspace", cwd: "/repo" },
      controller.signal,
    );
    controller.abort(reason);

    await expect(opening).rejects.toBe(reason);
    expect(subscribe).not.toHaveBeenCalled();
  });

  it("keeps global invalidations online when the active workspace disappeared", async () => {
    resolveWorkspace.mockResolvedValue({
      ref: { path: "/missing/repo" },
      projectRoot: "/missing/repo",
      availability: "missing",
    });
    const signal = new AbortController().signal;

    await expect(
      subscribeRuntimeWorkspaceEvents({ type: "workspace", cwd: "/missing/repo" }, signal),
    ).resolves.toBe(events);

    expect(subscribe).toHaveBeenCalledWith(
      expect.not.objectContaining({ watches: expect.anything() }),
      signal,
    );
  });

  it("does not resolve a watch scope when file watching is unavailable", async () => {
    fileWatch.mockReturnValue(false);
    const signal = new AbortController().signal;

    await expect(subscribeRuntimeWorkspaceEvents({ type: "workspace" }, signal)).resolves.toBe(
      events,
    );

    expect(resolveWorkspace).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(
      expect.not.objectContaining({ watches: expect.anything() }),
      signal,
    );
  });

  it("subscribes global topics without resolving a default watch while identity is unknown", async () => {
    const signal = new AbortController().signal;

    await expect(subscribeRuntimeWorkspaceEvents({ type: "none" }, signal)).resolves.toBe(events);

    expect(resolveWorkspace).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith(
      expect.not.objectContaining({ watches: expect.anything() }),
      signal,
    );
  });
});
