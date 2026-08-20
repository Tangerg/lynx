// The set of sessions held open, and which session the app should be on after a
// change to it.
//
// `activeSessionId` appears here as an INPUT read from the location and as an
// OUTPUT the caller navigates to — never as a field anyone stores. Closing the
// session you are looking at has to answer "then where?", and that answer is a
// move, not a write.

export interface AgentOpenSessions {
  activeSessionId: string;
  openSessionIds: string[];
}

/** The tab set after holding `sessionId` open. */
export function openSession(openSessionIds: string[], sessionId: string): string[] {
  return openSessionIds.includes(sessionId) ? openSessionIds : [...openSessionIds, sessionId];
}

export function closeOpenSession(state: AgentOpenSessions, sessionId: string): AgentOpenSessions {
  const index = state.openSessionIds.indexOf(sessionId);
  const openSessionIds = state.openSessionIds.filter((id) => id !== sessionId);
  const leavingActive = sessionId === state.activeSessionId;
  return {
    openSessionIds,
    // The neighbour that slid into its place, else the last one, else nowhere.
    activeSessionId: leavingActive
      ? (openSessionIds[index] ?? openSessionIds.at(-1) ?? "")
      : state.activeSessionId,
  };
}

export function reconcileOpenSessions(
  state: AgentOpenSessions & { provisionalSessionIds: Set<string> },
  liveIds: string[],
): AgentOpenSessions | null {
  // Only an in-process create that has not reached the next authoritative list
  // may supplement Runtime membership. Persisted draft ownership controls UI
  // visibility, but cannot prove that another client did not delete the Session.
  const known = new Set([...liveIds, ...state.provisionalSessionIds]);
  const retainedOpenSessionIds = state.openSessionIds.filter((id) => known.has(id));
  const activeAlive = state.activeSessionId === "" || known.has(state.activeSessionId);
  // A URL deep-link or browser history move can select a Session without going
  // through the explicit selection action. Once Runtime confirms that identity,
  // hold it open before stale refs are released: open-set subscribers own the
  // mounted Agent/composer lifecycle and would otherwise prune the still-active
  // Session as dead while leaving its location untouched.
  const openSessionIds =
    activeAlive && state.activeSessionId !== ""
      ? openSession(retainedOpenSessionIds, state.activeSessionId)
      : retainedOpenSessionIds;
  const openSessionsChanged =
    openSessionIds.length !== state.openSessionIds.length ||
    openSessionIds.some((id, index) => id !== state.openSessionIds[index]);
  if (!openSessionsChanged && activeAlive) return null;
  return {
    openSessionIds,
    activeSessionId: activeAlive ? state.activeSessionId : (openSessionIds.at(-1) ?? ""),
  };
}

export function pruneDraftSessions(state: {
  openSessionIds: string[];
  draftSessionIds: Set<string>;
}): Set<string> | null {
  const live = new Set(state.openSessionIds);
  const draftSessionIds = new Set([...state.draftSessionIds].filter((id) => live.has(id)));
  return draftSessionIds.size === state.draftSessionIds.size ? null : draftSessionIds;
}
