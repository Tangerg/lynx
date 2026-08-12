import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { reportSessionError } from "./reportSessionError";

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
  firstInput,
  firstRunOptions,
  cwd,
}: CreateSessionOptions): Promise<string | null> {
  try {
    const session = await agentRuntime().createSession(cwd ? { cwd } : {});
    const store = agentSessionState();
    // Mark draft + queue the message BEFORE selecting, so the remount
    // useAgentSession triggers sees both already in place.
    store.markDraftSession(session.id);
    if (firstInput?.parts.length)
      store.setPendingMessage(session.id, { input: firstInput, runOptions: firstRunOptions ?? {} });
    store.selectSession(session.id); // opens + sets active → remounts chat
    // Draft is filtered out of the Work Index; refetch so its graduation
    // (and any backend-assigned title) lands promptly. A cwd create may
    // also have minted a brand-new project.
    void invalidateAgentSessions();
    return session.id;
  } catch (err) {
    reportSessionError("create", err);
    return null;
  }
}

// In-flight latch: every "New" entry point (rail "+", ⌘N, palette command) fires
// bare, and sessions.create is a full round-trip — a double-click inside that
// window would otherwise create two backend sessions. Re-entrant calls join the
// pending create instead.
//
// Keyed by the request, because "join the one in flight" is only right for the
// SAME request. It used to join any of them, so a create that carried a cwd (the
// project "+") could be handed a session in the runtime's default directory
// instead of the project's — and worse, a welcome-composer send that landed in
// the window got back someone else's session while its typed message was never
// queued anywhere. `chatSend` fires that create and never inspects the id, so the
// text simply vanished.
let inflight: Promise<string | null> | null = null;
let inflightKey: string | null = null;

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
function alreadyOnAFreshSession(opts: CreateSessionOptions): string | null {
  if (opts.cwd !== undefined || opts.firstInput) return null;
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId || !agentSessionState().isDraftSession(sessionId)) return null;
  const messages = agentSessionView().getSession(sessionId)?.view.messages ?? [];
  return messages.length === 0 ? sessionId : null;
}

function doCreate(opts: CreateSessionOptions): Promise<string | null> {
  const key = joinKey(opts);
  if (inflight && key !== null && key === inflightKey) return inflight;
  const fresh = alreadyOnAFreshSession(opts);
  if (fresh) return Promise.resolve(fresh);
  const pending = createAndOpen(opts).finally(() => {
    // A later create may already own the latch.
    if (inflight === pending) {
      inflight = null;
      inflightKey = null;
    }
  });
  if (key !== null) {
    inflight = pending;
    inflightKey = key;
  }
  return pending;
}

/** Imperative create for non-React callers (palette commands, keymap).
 *  React components use {@link useCreateSession}. */
export function createSession(): Promise<string | null> {
  return doCreate({});
}

export function useCreateSession(): (opts?: CreateSessionOptions) => Promise<string | null> {
  return useCallback((opts) => doCreate(opts ?? {}), []);
}
