import type { SidebarProject, SidebarSession } from "@/lib/data/queries";
import { basename } from "@/lib/path";
import type { WorkGroup, WorkProject, WorkSession } from "../domain/workIndex";

interface BuildWorkIndexGroupsInput {
  projects: readonly SidebarProject[] | undefined;
  sessions: readonly SidebarSession[];
  fallbackProjectName: string;
}

function compareTimeDesc(a: { time: string }, b: { time: string }): number {
  if (a.time === b.time) return 0;
  return a.time < b.time ? 1 : -1;
}

function compareProjectSession(a: SidebarSession, b: SidebarSession): number {
  if (Boolean(a.favorite) !== Boolean(b.favorite)) return a.favorite ? -1 : 1;
  return compareTimeDesc(a, b);
}

function toWorkSession(session: SidebarSession): WorkSession {
  return {
    id: session.id,
    title: session.title,
    status: session.status,
    model: session.model,
    cwd: session.cwd,
    cwdMissing: session.cwdMissing,
    usage: session.usage,
    favorite: session.favorite,
    time: session.time,
  };
}

function toWorkProject(project: SidebarProject): WorkProject {
  return {
    id: project.id,
    name: project.name,
    branch: project.branch,
    sessionCount: project.sessionCount,
    cwdMissing: project.cwdMissing,
  };
}

export function buildWorkIndexGroups({
  projects,
  sessions,
  fallbackProjectName,
}: BuildWorkIndexGroupsInput): WorkGroup[] | undefined {
  if (!projects && sessions.length === 0) return undefined;

  const sessionsByCwd = new Map<string, SidebarSession[]>();
  for (const session of sessions) {
    const key = session.cwd ?? "";
    const group = sessionsByCwd.get(key);
    if (group) group.push(session);
    else sessionsByCwd.set(key, [session]);
  }

  for (const group of sessionsByCwd.values()) group.sort(compareProjectSession);

  const groups: WorkGroup[] = (projects ?? []).map((project) => {
    const sessions = sessionsByCwd.get(project.id) ?? [];
    sessionsByCwd.delete(project.id);
    return {
      project: toWorkProject(project),
      sessions: sessions.map(toWorkSession),
    };
  });

  for (const [cwd, sessions] of sessionsByCwd) {
    groups.push({
      project: {
        id: cwd,
        name: cwd ? basename(cwd) : fallbackProjectName,
        branch: "",
        sessionCount: sessions.length,
      },
      sessions: sessions.map(toWorkSession),
    });
  }

  return groups;
}

export function buildRecentWorkSessions(
  sessions: readonly SidebarSession[],
  limit: number,
): WorkSession[] {
  return [...sessions].sort(compareTimeDesc).slice(0, limit).map(toWorkSession);
}
