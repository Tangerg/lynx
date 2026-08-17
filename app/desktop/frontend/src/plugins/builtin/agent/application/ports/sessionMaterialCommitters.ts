/** Published extension point for read models carried beside the Agent projection
 * in one authoritative material response. Contributors receive their own
 * boundary type; the registry erases it without teaching Agent Application
 * about Runtime DTOs or another bounded context's state. */
export type AgentSessionMaterialCommitter<T> = (
  sessionId: string,
  material: T,
) => (() => void) | undefined;

interface RegisteredCommitter {
  active: boolean;
  stage(sessionId: string, material: unknown): (() => void) | undefined;
}

const committers = new Set<RegisteredCommitter>();

export function registerAgentSessionMaterialCommitter<T>(
  committer: AgentSessionMaterialCommitter<T>,
): () => void {
  const registered: RegisteredCommitter = {
    active: true,
    stage: (sessionId, material) => committer(sessionId, material as T),
  };
  committers.add(registered);
  return () => {
    registered.active = false;
    committers.delete(registered);
  };
}

export function stageAgentSessionMaterialCommits<T>(sessionId: string, material: T): () => void {
  const staged = [...committers].flatMap((registered) => {
    const commit = registered.stage(sessionId, material);
    return commit ? [{ registered, commit }] : [];
  });
  return () => {
    for (const { registered, commit } of staged) {
      if (registered.active) commit();
    }
  };
}
