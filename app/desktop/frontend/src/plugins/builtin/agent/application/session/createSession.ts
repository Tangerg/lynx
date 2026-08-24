import { queryClient } from "@/lib/queryClient";
import { useCallback } from "react";
import {
  AGENT_SESSIONS_KEY,
  invalidateAgentSessions,
  type AgentSessionSummary,
} from "./sessionQueries";
import { agentRuntime, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { agentSessionState, type AgentSessionStatePort } from "../ports/sessionState";
import { agentSessionView, type AgentSessionViewPort } from "../ports/sessionView";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner, type AgentCommandOwner } from "../agentCommandOwner";

export interface CreateSessionOptions {
  /** Create the session in this exact working directory. Desktop never delegates
   *  an omitted workspace to the Runtime process default: project selection is
   *  an explicit user gesture, and Session owns the resulting cwd. */
  cwd: string;
  /** Treat the current empty draft as the requested destination even when cwd
   *  is supplied. The top-level New action already knows that cwd belongs to
   *  the active Session; project-row creation does not make that claim. */
  reuseFreshDraft?: boolean;
}

/**
 * Create a fresh backend session as a hidden **draft**, open it as the
 * active session. Returns the new id (or null if the create failed).
 *
 * A draft is a real session (so runs.start works immediately) that stays
 * out of the visible Work Index until its first message graduates it. Project
 * selection creates the draft first; only the mounted Session lifecycle may
 * subsequently accept the composer's input.
 */
async function createAndOpen({
  owner,
  runtime,
  state,
  cwd,
}: CreateSessionOptions & {
  owner: AgentCommandOwner;
  runtime: AgentRuntimeGateway;
  state: AgentSessionStatePort;
}): Promise<string> {
  const session = await runtime.createSession({ cwd });
  owner.assertCurrent();
  // Mark the Session as a draft before selecting it so the mounted lifecycle
  // can safely skip a durable read for this same-process empty identity.
  state.markDraftSession(session.id);
  state.selectSession(session.id); // opens + sets active → remounts chat
  // Draft is filtered out of the Work Index; refetch so its graduation
  // (and any backend-assigned title) lands promptly. A cwd create may
  // also have minted a brand-new project.
  void invalidateAgentSessions();
  return session.id;
}

// Only exact workspace destinations may share an in-flight create. Requests for
// different projects must never receive one another's Session identity.
function joinKey(opts: CreateSessionOptions): string {
  return `cwd:${opts.cwd}`;
}

/**
 * The fresh session the user is already looking at, if any.
 *
 * "New session" asks to be put in front of an empty composer — it is a
 * destination, not an instruction to allocate. The empty-composer screen is any
 * active session with no messages, so selecting that destination is a no-op when
 * the user is already there.
 *
 * Only a DRAFT counts. An ordinary session also reads as message-less while its
 * history is still loading, and reusing that would drop the user back into a
 * conversation they asked to leave. A cwd create also creates unless the caller
 * proves it is reopening the active project's blank destination through
 * `reuseFreshDraft`.
 */
function alreadyOnAFreshSession(
  opts: CreateSessionOptions,
  state: AgentSessionStatePort,
  view: AgentSessionViewPort,
): string | null {
  if (!opts.reuseFreshDraft) return null;
  const sessionId = state.getActiveSessionId();
  if (!sessionId || !state.isDraftSession(sessionId)) return null;
  const messages = view.getSession(sessionId)?.view.messages ?? [];
  return messages.length === 0 ? sessionId : null;
}

function doCreate(opts: CreateSessionOptions): Promise<string | null> {
  if (opts.cwd.trim() === "") return Promise.resolve(null);
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const state = agentSessionState();
  const view = agentSessionView();
  const key = joinKey(opts);
  const fresh = alreadyOnAFreshSession(opts, state, view);
  if (fresh) return Promise.resolve(fresh);
  return owner
    .runSessionCreate(key, () => createAndOpen({ owner, runtime, state, ...opts }))
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError("create", error);
      return null;
    });
}

/** Imperative New for non-React callers (palette commands, keymap).
 *
 * The active Session is the only authoritative source of the inherited cwd.
 * If no Session is active, or its summary has not resolved, New is a focus move
 * to the project-selection destination rather than a backend mutation.
 */
export function createSession(): Promise<string | null> {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return Promise.resolve(null);
  const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
  const cwd = sessions?.find((session) => session.id === sessionId)?.workspace.path;
  if (!cwd || cwd.trim() === "") return Promise.resolve(null);
  return doCreate({ cwd, reuseFreshDraft: true });
}

export function useCreateSession(): (opts: CreateSessionOptions) => Promise<string | null> {
  return useCallback((opts) => doCreate(opts), []);
}
