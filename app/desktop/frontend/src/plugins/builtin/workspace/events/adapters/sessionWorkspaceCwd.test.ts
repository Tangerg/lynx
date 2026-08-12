import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/plugins/builtin/agent/public/session";
import { resolveActiveSessionWorkspaceCwd } from "./sessionWorkspaceCwd";

const { activeSessionId, getSession } = vi.hoisted(() => ({
  activeSessionId: vi.fn<() => string | null>(() => null),
  getSession: vi.fn(),
}));

vi.mock("@/plugins/builtin/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/agent/public/session")>()),
  getActiveSessionId: activeSessionId,
}));

vi.mock("@/main/container", () => ({
  getContainer: () => ({
    client: () => ({ sessions: { get: getSession } }),
  }),
}));

afterEach(() => {
  activeSessionId.mockReturnValue(null);
  getSession.mockReset();
  queryClient.removeQueries({ queryKey: [AGENT_SESSIONS_KEY] });
});

describe("active session workspace resolution", () => {
  it("resolves no active session to the runtime default workspace", async () => {
    await expect(resolveActiveSessionWorkspaceCwd()).resolves.toEqual({ status: "resolved" });
    expect(getSession).not.toHaveBeenCalled();
  });

  it("uses the matching session-list projection", async () => {
    activeSessionId.mockReturnValue("ses_cached");
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [{ id: "ses_cached", cwd: "/cached/repo" }]);

    await expect(resolveActiveSessionWorkspaceCwd()).resolves.toEqual({
      status: "resolved",
      cwd: "/cached/repo",
    });
    expect(getSession).not.toHaveBeenCalled();
  });

  it("reads a draft session which is not present in the list projection", async () => {
    activeSessionId.mockReturnValue("ses_draft");
    queryClient.setQueryData([AGENT_SESSIONS_KEY], []);
    getSession.mockResolvedValue({ workspace: { ref: { path: "/draft/repo" } } });

    await expect(resolveActiveSessionWorkspaceCwd()).resolves.toEqual({
      status: "resolved",
      cwd: "/draft/repo",
    });
    expect(getSession).toHaveBeenCalledWith("ses_draft");
  });

  it("reports an unavailable authoritative read for subscription retry", async () => {
    activeSessionId.mockReturnValue("ses_remote");
    getSession.mockRejectedValue(new Error("offline"));

    await expect(resolveActiveSessionWorkspaceCwd()).resolves.toEqual({
      status: "unavailable",
    });
  });
});
