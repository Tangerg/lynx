import { describe, expect, it } from "vitest";
import type { SidebarProject, SidebarSession } from "@/lib/data/queries";
import { buildRecentWorkSessions, buildWorkIndexGroups } from "./buildWorkIndex";

function session(overrides: Partial<SidebarSession> & Pick<SidebarSession, "id">): SidebarSession {
  return {
    title: overrides.id,
    status: "idle",
    model: "gpt-test",
    time: "2026-01-01T00:00:00.000Z",
    ...overrides,
  };
}

function project(overrides: Partial<SidebarProject> & Pick<SidebarProject, "id">): SidebarProject {
  return {
    name: overrides.id,
    branch: "",
    sessionCount: 0,
    ...overrides,
  };
}

describe("buildWorkIndexGroups", () => {
  it("returns undefined while both projects and sessions are absent", () => {
    expect(
      buildWorkIndexGroups({
        projects: undefined,
        sessions: [],
        fallbackProjectName: "Other",
      }),
    ).toBeUndefined();
  });

  it("groups sessions by cwd and keeps unmatched cwd sessions reachable", () => {
    const groups = buildWorkIndexGroups({
      projects: [project({ id: "/repo/lynx", name: "lynx", sessionCount: 2 })],
      sessions: [
        session({ id: "a", cwd: "/repo/lynx", time: "2026-01-01T00:00:00.000Z" }),
        session({ id: "b", cwd: "/repo/planet_new", time: "2026-01-02T00:00:00.000Z" }),
      ],
      fallbackProjectName: "Other",
    });

    expect(groups?.map((group) => group.project.id)).toEqual(["/repo/lynx", "/repo/planet_new"]);
    expect(groups?.[0]?.sessions.map((item) => item.id)).toEqual(["a"]);
    expect(groups?.[1]?.project.name).toBe("planet_new");
    expect(groups?.[1]?.sessions.map((item) => item.id)).toEqual(["b"]);
  });

  it("pins favorite sessions inside their project before recency sorting", () => {
    const groups = buildWorkIndexGroups({
      projects: [project({ id: "/repo/lynx", name: "lynx" })],
      sessions: [
        session({ id: "recent", cwd: "/repo/lynx", time: "2026-01-03T00:00:00.000Z" }),
        session({
          id: "favorite",
          cwd: "/repo/lynx",
          favorite: true,
          time: "2026-01-01T00:00:00.000Z",
        }),
        session({ id: "middle", cwd: "/repo/lynx", time: "2026-01-02T00:00:00.000Z" }),
      ],
      fallbackProjectName: "Other",
    });

    expect(groups?.[0]?.sessions.map((item) => item.id)).toEqual(["favorite", "recent", "middle"]);
  });

  it("uses a fallback group for sessions without cwd", () => {
    const groups = buildWorkIndexGroups({
      projects: [],
      sessions: [session({ id: "chat-only" })],
      fallbackProjectName: "Other",
    });

    expect(groups).toEqual([
      {
        project: { id: "", name: "Other", branch: "", sessionCount: 1 },
        sessions: [
          {
            id: "chat-only",
            title: "chat-only",
            status: "idle",
            model: "gpt-test",
            cwd: undefined,
            cwdMissing: undefined,
            usage: undefined,
            favorite: undefined,
            time: "2026-01-01T00:00:00.000Z",
          },
        ],
      },
    ]);
  });
});

describe("buildRecentWorkSessions", () => {
  it("creates a newest-first recent list without mutating the source array", () => {
    const sessions = [
      session({ id: "old", time: "2026-01-01T00:00:00.000Z" }),
      session({ id: "new", time: "2026-01-03T00:00:00.000Z" }),
      session({ id: "middle", time: "2026-01-02T00:00:00.000Z" }),
    ];

    expect(buildRecentWorkSessions(sessions, 2).map((item) => item.id)).toEqual(["new", "middle"]);
    expect(sessions.map((item) => item.id)).toEqual(["old", "new", "middle"]);
  });
});
