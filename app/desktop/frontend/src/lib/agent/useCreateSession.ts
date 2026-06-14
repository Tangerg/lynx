import type { ContentBlock } from "@/rpc";
import { useCallback } from "react";
import { getContainer } from "@/main/container";
import { invalidateSessions } from "@/lib/data/queries";
import { useSessionStore } from "@/state/sessionStore";
import { reportSessionError } from "./reportSessionError";

export interface CreateSessionOptions {
  /** Queue this as the session's first message input (welcome composer). */
  firstInput?: ContentBlock[];
  /** Create the session in this working directory (sessions.create cwd,
   *  API.md §7.2) — the Projects "+" / project-row entry. Omitted = the
   *  runtime's serve dir. */
  cwd?: string;
}

/**
 * Create a fresh backend session as a hidden **draft**, open it as the
 * active tab, and optionally queue its first message. Returns the new id
 * (or null if the create failed).
 *
 * A draft is a real session (so runs.start works immediately) that stays
 * out of the sidebar list until its first message graduates it — the
 * ChatGPT/Claude/Proma pattern. The "New" button calls this with no text
 * (an empty draft ready to type into); the welcome composer calls it with
 * the typed text, which the chat flushes on remount (useAgentSession).
 */
async function createAndOpen({ firstInput, cwd }: CreateSessionOptions): Promise<string | null> {
  try {
    const session = await getContainer()
      .client()
      .sessions.create(cwd ? { cwd } : {});
    const store = useSessionStore.getState();
    // Mark draft + queue the message BEFORE selecting, so the remount
    // useAgentSession triggers sees both already in place.
    store.markDraft(session.id);
    if (firstInput?.length) store.setPendingMessage(session.id, firstInput);
    store.selectTab(session.id); // opens tab + sets active → remounts chat
    // Draft is filtered out of the sidebar; refetch so its graduation
    // (and any backend-assigned title) lands promptly. A cwd create may
    // also have minted a brand-new project.
    void invalidateSessions(cwd ? { projects: true } : undefined);
    return session.id;
  } catch (err) {
    reportSessionError("create", err);
    return null;
  }
}

// In-flight latch: every "New" entry point (rail "+", ⌘N, palette command,
// welcome composer) fires bare, and sessions.create is a full round-trip — a
// double-click inside that window would otherwise create two backend sessions
// and two tabs. Re-entrant calls join the pending create instead.
let inflight: Promise<string | null> | null = null;

function doCreate(opts: CreateSessionOptions): Promise<string | null> {
  if (inflight) return inflight;
  inflight = createAndOpen(opts).finally(() => {
    inflight = null;
  });
  return inflight;
}

/** Imperative create for non-React callers (palette commands, keymap).
 *  React components use {@link useCreateSession}. */
export function createSession(): Promise<string | null> {
  return doCreate({});
}

export function useCreateSession(): (opts?: CreateSessionOptions) => Promise<string | null> {
  return useCallback((opts) => doCreate(opts ?? {}), []);
}
