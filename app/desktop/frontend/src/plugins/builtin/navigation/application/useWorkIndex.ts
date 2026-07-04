import { useMemo } from "react";
import { useProjects } from "@/lib/data/queries";
import {
  useActiveSessionCwd,
  useActiveSessionId,
  useVisibleAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import type { WorkIndex } from "../domain/workIndex";
import { buildWorkIndexGroups } from "./buildWorkIndex";

interface UseWorkIndexOptions {
  fallbackProjectName: string;
}

export function useWorkIndex({ fallbackProjectName }: UseWorkIndexOptions): WorkIndex {
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

  return {
    groups,
    activeSessionId,
    activeCwd,
    isLoading: projects.isLoading && !groups,
    isError: projects.isError && !groups,
  };
}
