import { useMemo } from "react";
import { useProjects } from "@/lib/data/queries";
import {
  useActiveSessionCwd,
  useActiveSessionId,
  useVisibleAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import type { WorkIndex } from "../domain/workIndex";
import { buildRecentWorkSessions, buildWorkIndexGroups } from "./buildWorkIndex";

interface UseWorkIndexOptions {
  fallbackProjectName: string;
  recentLimit?: number;
}

export function useWorkIndex({
  fallbackProjectName,
  recentLimit = 5,
}: UseWorkIndexOptions): WorkIndex {
  const projects = useProjects();
  const sessions = useVisibleAgentSessions();
  const activeSessionId = useActiveSessionId();
  const activeCwd = useActiveSessionCwd();

  const groups = useMemo(
    () =>
      buildWorkIndexGroups({
        projects: projects.data,
        sessions,
        fallbackProjectName,
      }),
    [projects.data, sessions, fallbackProjectName],
  );
  const recentSessions = useMemo(
    () => buildRecentWorkSessions(sessions, recentLimit),
    [sessions, recentLimit],
  );

  return {
    groups,
    recentSessions,
    activeSessionId,
    activeCwd,
    isLoading: projects.isLoading && !groups,
    isError: projects.isError && !groups,
  };
}

export function useRecentWorkSessions(limit = 5) {
  const sessions = useVisibleAgentSessions();
  const activeSessionId = useActiveSessionId();
  const recentSessions = useMemo(() => buildRecentWorkSessions(sessions, limit), [sessions, limit]);
  return { activeSessionId, recentSessions };
}
