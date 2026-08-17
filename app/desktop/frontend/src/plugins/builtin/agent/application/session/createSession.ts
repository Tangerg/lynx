import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime, type AgentRuntimeGateway } from "../ports/runtimeGateway";
import { agentSessionState, type AgentSessionStatePort } from "../ports/sessionState";
import { agentSessionView, type AgentSessionViewPort } from "../ports/sessionView";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner, type AgentCommandOwner } from "../agentCommandOwner";

export interface CreateSessionOptions {
  /** Queue this as the session's first message input (welcome composer). */
  firstInput?: AgentInput;
  /** Run options bound to firstInput. */
  firstRunOptions?: AgentRunStartOptions;
  /** Create the session in this working directory (sessions.create cwd,
   *  API.md §7.2) — the Projects "+" / project-row entry. Omitted = the
   *  runtime's serve dir. */
  cwd?: string;
}

/**
 * Create a fresh backend session as a hidden **draft**, open it as the
 * active session, and optionally queue its first message. Returns the new id
 * (or null if the create failed).
 *
 * A draft is a real session (so runs.start works immediately) that stays
 * out of the session summary list until its first message graduates it — the
 * ChatGPT/Claude/Proma pattern. The "New" button calls this with no text
 * (an empty draft ready to type into); the welcome composer calls it with
 * the typed text, which the chat flushes on remount (useAgentSession).
 */
async function createAndOpen({
  owner,
  runtime,
  state,
  firstInput,
  firstRunOptions,
  cwd,
}: CreateSessionOptions & {
  owner: AgentCommandOwner;
  runtime: AgentRuntimeGateway;
  state: AgentSessionStatePort;
}): Promise<string | null> {
  try {
    const session = await runtime.createSession(cwd ? { cwd } : {});
    if (!owner.isCurrent()) return null;
    // Mark draft + queue the message BEFORE selecting, so the remount
    // useAgentSession triggers sees both already in place.
    state.markDraftSession(session.id);
    if (firstInput?.parts.length)
      state.setPendingMessage(session.id, { input: firstInput, runOptions: firstRunOptions ?? {} });
    state.selectSession(session.id); // opens + sets active → remounts chat
    // Draft is filtered out of the Work Index; refetch so its graduation
    // (and any backend-assigned title) lands promptly. A cwd create may
    // also have minted a brand-new project.
    void invalidateAgentSessions();
    return session.id;
  } catch (err) {
    if (owner.isCurrent()) reportSessionError("create", err);
    return null;
  }
}

// Keyed by the request, because "join the one in flight" is only right for the
// SAME request. It used to join any of them, so a create that carried a cwd (the
// project "+") could be handed a session in the runtime's default directory
// instead of the project's — and worse, a welcome-composer send that landed in
// the window got back someone else's session while its typed message was never
// queued anywhere. `chatSend` fires that create and never inspects the id, so the
// text simply vanished.
/** The requests that may share one create, or null for one that may not: a queued
 *  first message belongs to exactly one session. */
function joinKey(opts: CreateSessionOptions): string | null {
  return opts.firstInput ? null : `cwd:${opts.cwd ?? ""}`;
}

/**
 * The fresh session the user is already looking at, if any.
 *
 * "New session" asks to be put in front of an empty composer — it is a
 * destination, not an instruction to allocate. The empty-composer screen is any
 * active session with no messages, so pressing "New" while already there used to
 * mint a second backend session that looked exactly the same, and the first one
 * stayed on the runtime as a draft the session list filters out: invisible, and
 * one more per press.
 *
 * Only a DRAFT counts. An ordinary session also reads as message-less while its
 * history is still loading, and reusing that would drop the user back into a
 * conversation they asked to leave. A `cwd` or a queued first message means the
 * caller wants a specific session, not just a blank one, so those always create.
 */
function alreadyOnAFreshSession(
  opts: CreateSessionOptions,
  state: AgentSessionStatePort,
  view: AgentSessionViewPort,
): string | null {
  if (opts.cwd !== undefined || opts.firstInput) return null;
  const sessionId = state.getActiveSessionId();
  if (!sessionId || !state.isDraftSession(sessionId)) return null;
  const messages = view.getSession(sessionId)?.view.messages ?? [];
  return messages.length === 0 ? sessionId : null;
}

function doCreate(opts: CreateSessionOptions): Promise<string | null> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const state = agentSessionState();
  const view = agentSessionView();
  const key = joinKey(opts);
  const fresh = alreadyOnAFreshSession(opts, state, view);
  if (fresh) return Promise.resolve(fresh);
  return owner.runSessionCreate(key, () => createAndOpen({ owner, runtime, state, ...opts }));
}

/** Imperative create for non-React callers (palette commands, keymap).
 *  React components use {@link useCreateSession}. */
export function createSession(): Promise<string | null> {
  return doCreate({});
}

export function useCreateSession(): (opts?: CreateSessionOptions) => Promise<string | null> {
  return useCallback((opts) => doCreate(opts ?? {}), []);
}
