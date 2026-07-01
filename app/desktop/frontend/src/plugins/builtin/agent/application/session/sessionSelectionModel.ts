export interface AgentSessionTabs {
  activeSessionId: string;
  tabIds: string[];
}

export interface AgentSessionSelection extends AgentSessionTabs {
  selectionEpoch: number;
}

export function selectSessionTab(
  state: AgentSessionSelection,
  sessionId: string,
): AgentSessionSelection {
  return {
    activeSessionId: sessionId,
    selectionEpoch: state.selectionEpoch + 1,
    tabIds: state.tabIds.includes(sessionId) ? state.tabIds : [...state.tabIds, sessionId],
  };
}

export function closeSessionTab(state: AgentSessionTabs, sessionId: string): AgentSessionTabs {
  const index = state.tabIds.indexOf(sessionId);
  const tabIds = state.tabIds.filter((id) => id !== sessionId);
  const leavingActive = sessionId === state.activeSessionId;
  return {
    tabIds,
    activeSessionId: leavingActive ? (tabIds[index] ?? tabIds.at(-1) ?? "") : state.activeSessionId,
  };
}

export function reconcileSessionTabs(
  state: AgentSessionTabs & { draftSessionIds: Set<string> },
  liveIds: string[],
): AgentSessionTabs | null {
  const known = new Set([...liveIds, ...state.draftSessionIds]);
  const tabIds = state.tabIds.filter((id) => known.has(id));
  const activeAlive = state.activeSessionId === "" || known.has(state.activeSessionId);
  if (tabIds.length === state.tabIds.length && activeAlive) return null;
  return {
    tabIds,
    activeSessionId: activeAlive ? state.activeSessionId : (tabIds.at(-1) ?? ""),
  };
}

export function pruneSessionHandoffs<T>(state: {
  tabIds: string[];
  draftSessionIds: Set<string>;
  pendingMessages: Record<string, T>;
}): { draftSessionIds: Set<string>; pendingMessages: Record<string, T> } | null {
  const live = new Set(state.tabIds);
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
