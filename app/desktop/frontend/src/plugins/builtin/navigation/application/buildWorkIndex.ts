import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import type { WorkspaceProjectSummary } from "@/plugins/builtin/workspace/public/queries";
import type { WorkGroup, WorkIndexContent, WorkProject, WorkSession } from "../domain/workIndex";

interface BuildWorkIndexInput {
  projects: readonly WorkspaceProjectSummary[] | undefined;
  sessions: readonly AgentSessionSummary[];
}

function compareTimeDesc(a: { time: string }, b: { time: string }): number {
  if (a.time === b.time) return 0;
  return a.time < b.time ? 1 : -1;
}

function compareProjectSession(a: AgentSessionSummary, b: AgentSessionSummary): number {
  if (Boolean(a.favorite) !== Boolean(b.favorite)) return a.favorite ? -1 : 1;
  return compareTimeDesc(a, b);
}

function toWorkSessionAttention(session: AgentSessionSummary): WorkSession["attention"] {
  if (session.status === "running") return "running";
  if (session.status === "waiting") return "waiting";
  return "none";
}

function toWorkSession(session: AgentSessionSummary): WorkSession {
  return {
    id: session.id,
    revision: session.revision,
    title: session.title,
    attention: toWorkSessionAttention(session),
    favorite: session.favorite,
    time: session.time,
  };
}

function toWorkProject(project: WorkspaceProjectSummary): WorkProject {
  return {
    id: project.id,
    name: project.name,
    cwdMissing: project.cwdMissing,
  };
}

export function buildWorkIndex({
  projects,
  sessions,
}: BuildWorkIndexInput): WorkIndexContent | undefined {
  if (!projects && sessions.length === 0) return undefined;

  const byDirectory = new Map<string, AgentSessionSummary[]>();
  for (const session of sessions) {
    const key = session.workspace.path;
    const directory = byDirectory.get(key);
    if (directory) directory.push(session);
    else byDirectory.set(key, [session]);
  }

  const groups: WorkGroup[] = (projects ?? []).map((project) => {
    const owned = byDirectory.get(project.id) ?? [];
    byDirectory.delete(project.id);
    return {
      project: toWorkProject(project),
      sessions: owned.sort(compareProjectSession).map(toWorkSession),
    };
  });

  // Recency alone, unlike a project's list: a section called "Recent" whose top
  // row is three weeks old because someone pinned it is not a recent list.
  // Pinning still shows on the row, and it still orders the project it lives in.
  const recents = [...byDirectory.values()].flat().sort(compareTimeDesc).map(toWorkSession);

  return { groups, recents };
}
