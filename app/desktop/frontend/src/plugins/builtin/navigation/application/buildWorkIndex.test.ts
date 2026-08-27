import { describe, expect, it } from "vitest";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type { WorkspaceProjectSummary } from "@/plugins/builtin/workspace/public/queries";
import { buildWorkIndex } from "./buildWorkIndex";

type SessionOverrides = Omit<Partial<AgentSessionSummary>, "workspace"> &
  Pick<AgentSessionSummary, "id"> & { cwd?: string };

function session(overrides: SessionOverrides): AgentSessionSummary {
  const { cwd = "/unclaimed", ...summary } = overrides;
  return {
    revision: 1,
    title: overrides.id,
    status: "idle",
    provider: "provider",
    model: "gpt-test",
    workspace: { path: cwd, availability: "available" },
    time: "2026-01-01T00:00:00.000Z",
    ...summary,
  };
}

function project(
  overrides: Partial<WorkspaceProjectSummary> & Pick<WorkspaceProjectSummary, "id">,
): WorkspaceProjectSummary {
  return {
    name: overrides.id,
    sessionCount: 0,
    ...overrides,
  };
}

describe("buildWorkIndex", () => {
  it("returns undefined while both projects and sessions are absent", () => {
    expect(buildWorkIndex({ projects: undefined, sessions: [] })).toBeUndefined();
  });

  it("groups sessions under the project that owns their directory", () => {
    const content = buildWorkIndex({
      projects: [project({ id: "/repo/scope", name: "scope", sessionCount: 1 })],
      sessions: [session({ id: "a", cwd: "/repo/scope" })],
    });

    expect(content?.groups.map((group) => group.project.id)).toEqual(["/repo/scope"]);
    expect(content?.groups[0]?.sessions.map((item) => item.id)).toEqual(["a"]);
    expect(content?.recents).toEqual([]);
  });

  it("keeps an empty project visible rather than dropping it", () => {
    const content = buildWorkIndex({
      projects: [project({ id: "/repo/scope", name: "scope" })],
      sessions: [],
    });

    expect(content?.groups.map((group) => group.project.id)).toEqual(["/repo/scope"]);
    expect(content?.groups[0]?.sessions).toEqual([]);
  });

  it("sends sessions no project claims to recents, newest first", () => {
    const content = buildWorkIndex({
      projects: [project({ id: "/repo/scope", name: "scope" })],
      sessions: [
        session({ id: "owned", cwd: "/repo/scope", time: "2026-01-04T00:00:00.000Z" }),
        session({ id: "scratch", cwd: "/tmp/probe", time: "2026-01-02T00:00:00.000Z" }),
        session({ id: "loose", cwd: "/tmp/other", time: "2026-01-03T00:00:00.000Z" }),
        session({ id: "other", cwd: "/tmp/other-2", time: "2026-01-01T00:00:00.000Z" }),
      ],
    });

    expect(content?.groups[0]?.sessions.map((item) => item.id)).toEqual(["owned"]);
    expect(content?.recents.map((item) => item.id)).toEqual(["loose", "scratch", "other"]);
  });

  it("pins favorite sessions inside their project before recency sorting", () => {
    const content = buildWorkIndex({
      projects: [project({ id: "/repo/scope", name: "scope" })],
      sessions: [
        session({ id: "recent", cwd: "/repo/scope", time: "2026-01-03T00:00:00.000Z" }),
        session({
          id: "favorite",
          cwd: "/repo/scope",
          favorite: true,
          time: "2026-01-01T00:00:00.000Z",
        }),
        session({ id: "middle", cwd: "/repo/scope", time: "2026-01-02T00:00:00.000Z" }),
      ],
    });

    expect(content?.groups[0]?.sessions.map((item) => item.id)).toEqual([
      "favorite",
      "recent",
      "middle",
    ]);
  });

  it("orders recents by time even when one is pinned", () => {
    const content = buildWorkIndex({
      projects: [],
      sessions: [
        session({ id: "newer", time: "2026-01-03T00:00:00.000Z" }),
        session({ id: "pinned", favorite: true, time: "2026-01-01T00:00:00.000Z" }),
      ],
    });

    expect(content?.recents.map((item) => item.id)).toEqual(["newer", "pinned"]);
  });

  it("projects source status into work attention", () => {
    const content = buildWorkIndex({
      projects: [],
      sessions: [
        session({ id: "running", status: "running", time: "2026-01-03T00:00:00.000Z" }),
        session({ id: "waiting", status: "waiting", time: "2026-01-02T00:00:00.000Z" }),
        session({ id: "idle", status: "idle", time: "2026-01-01T00:00:00.000Z" }),
      ],
    });

    expect(content?.recents.map((item) => [item.id, item.attention])).toEqual([
      ["running", "running"],
      ["waiting", "waiting"],
      ["idle", "none"],
    ]);
  });
});
