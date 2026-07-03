export interface AgentOpenSessions {
  activeSessionId: string;
  openSessionIds: string[];
}

export interface AgentSessionSelection extends AgentOpenSessions {
  selectionEpoch: number;
}

export function selectOpenSession(
  state: AgentSessionSelection,
  sessionId: string,
): AgentSessionSelection {
  return {
    activeSessionId: sessionId,
    selectionEpoch: state.selectionEpoch + 1,
    openSessionIds: state.openSessionIds.includes(sessionId)
      ? state.openSessionIds
      : [...state.openSessionIds, sessionId],
  };
}

export function closeOpenSession(state: AgentOpenSessions, sessionId: string): AgentOpenSessions {
  const index = state.openSessionIds.indexOf(sessionId);
  const openSessionIds = state.openSessionIds.filter((id) => id !== sessionId);
  const leavingActive = sessionId === state.activeSessionId;
  return {
    openSessionIds,
    activeSessionId: leavingActive
      ? (openSessionIds[index] ?? openSessionIds.at(-1) ?? "")
      : state.activeSessionId,
  };
}

export function reconcileOpenSessions(
  state: AgentOpenSessions & { draftSessionIds: Set<string> },
  liveIds: string[],
): AgentOpenSessions | null {
  const known = new Set([...liveIds, ...state.draftSessionIds]);
  const openSessionIds = state.openSessionIds.filter((id) => known.has(id));
  const activeAlive = state.activeSessionId === "" || known.has(state.activeSessionId);
  if (openSessionIds.length === state.openSessionIds.length && activeAlive) return null;
  return {
    openSessionIds,
    activeSessionId: activeAlive ? state.activeSessionId : (openSessionIds.at(-1) ?? ""),
  };
}

export function pruneSessionHandoffs<T>(state: {
  openSessionIds: string[];
  draftSessionIds: Set<string>;
  pendingMessages: Record<string, T>;
}): { draftSessionIds: Set<string>; pendingMessages: Record<string, T> } | null {
  const live = new Set(state.openSessionIds);
  const draftSessionIds = new Set([...state.draftSessionIds].filter((id) => live.has(id)));
  const pendingMessages = Object.fromEntries(
    Object.entries(state.pendingMessages).filter(([id]) => live.has(id)),
  ) as Record<string, T>;
  if (
    draftSessionIds.size === state.draftSessionIds.size &&
    Object.keys(pendingMessages).length === Object.keys(state.pendingMessages).length
  ) {
    return null;
  }
  return { draftSessionIds, pendingMessages };
}
