// Agent session memory: which sessions are held open, draft-session
// bookkeeping, and the session to reopen on a cold start.
//
// WHICH SESSION IS ACTIVE IS NOT HERE. That is the app's location (see
// lib/navigation) so that history holds it. What is here is memory: the tab set,
// and `lastSessionId` — written as the user moves, read once at boot to seed the
// location. One direction only; nothing here is a second copy of the location.
//
// Persistence policy:
//   - Persisted: openSessionIds + lastSessionId + draftSessionIds (continuity
//     and ownership of provisional backend Sessions across launches).
//   - Ephemeral: freshDraftSessionIds. It proves that an in-process create may
//     skip the first durable read.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";
import { discardOlderVersions } from "@/lib/persistedStore";
import { openSession, pruneDraftSessions } from "../application/session/sessionSelectionModel";

// localStorage payload schema. Mirrors `partialize` below — only the
// continuity fields. Anything else in storage is dropped on rehydrate; a
// malformed entry falls back to defaults instead of crashing the boot.
const sessionPersistSchema = z.object({
  lastSessionId: z.string(),
  openSessionIds: z.array(z.string()),
  draftSessionIds: z.array(z.string()),
});

interface AgentSessionState {
  /** The set of sessions currently held open. This is load-bearing lifecycle
   *  state: agentStore drops view state, composerStore drops drafts, and this
   *  store drops draft refs for ids no longer in the set. */
  openSessionIds: string[];

  /** Where the user was when they last quit — the seed for a cold start, and
   *  nothing else. Reading it to answer "which session is active" would make it
   *  a second owner of the location. */
  lastSessionId: string;

  /**
   * Draft sessions — real backend sessions (created up front so they can
   * receive a run) that haven't had their first message yet. Hidden from
   * the Work Index until they "graduate" (first send), so a fresh
   * "New" doesn't litter the list with empties. Persisted until graduation so
   * a reload cannot publish an unused draft as an ordinary Session.
   */
  draftSessionIds: Set<string>;
  /** Drafts created in this process and therefore known to have no durable
   * history yet. Unlike draft ownership, this read-skipping fact is ephemeral. */
  freshDraftSessionIds: Set<string>;
}

interface AgentSessionActions {
  /** Hold a session open — the tab half of selecting it. */
  holdOpen: (id: string) => void;
  /** Drop a session from the open set. Where to go next is the caller's move. */
  release: (id: string) => void;
  /** Keep only these — boot reconciliation against the runtime's live ids. */
  retainOnly: (openSessionIds: string[]) => void;
  /** Record where the user is, for the next cold start. */
  rememberSession: (id: string) => void;

  /** Mark a session as a hidden draft (just created, no message yet). */
  markDraft: (id: string) => void;
  /** Promote a draft to a real session (first message sent). Idempotent. */
  graduateDraft: (id: string) => void;
}

export const useAgentSessionStore = create<AgentSessionState & AgentSessionActions>()(
  persist(
    (set, get) => ({
      // No demo fixtures — the open set starts empty and is driven by the real
      // backend's sessions.list (the sidebar) + user clicks. Ghost ids would
      // make the chat try to load/run a session the runtime doesn't have
      // (session_not_found on boot).
      openSessionIds: [],
      lastSessionId: "",
      draftSessionIds: new Set<string>(),
      freshDraftSessionIds: new Set<string>(),

      holdOpen: (id) => set({ openSessionIds: openSession(get().openSessionIds, id) }),
      release: (id) =>
        set({ openSessionIds: get().openSessionIds.filter((openId) => openId !== id) }),
      retainOnly: (openSessionIds) => set({ openSessionIds }),
      rememberSession: (id) => set({ lastSessionId: id }),
      markDraft: (id) =>
        set({
          draftSessionIds: new Set(get().draftSessionIds).add(id),
          freshDraftSessionIds: new Set(get().freshDraftSessionIds).add(id),
        }),
      graduateDraft: (id) => {
        const drafts = get().draftSessionIds;
        if (!drafts.has(id)) return;
        const next = new Set(drafts);
        next.delete(id);
        const fresh = new Set(get().freshDraftSessionIds);
        fresh.delete(id);
        set({ draftSessionIds: next, freshDraftSessionIds: fresh });
      },
    }),
    {
      name: "scopeapp.agent-session",
      storage: createJSONStorage(() => localStorage),
      // Persist continuity plus provisional Session ownership. Only the
      // in-process freshness proof is ephemeral.
      partialize: (s) => ({
        openSessionIds: s.openSessionIds,
        lastSessionId: s.lastSessionId,
        draftSessionIds: [...s.draftSessionIds],
      }),
      // Persisted shape is dev-phase only; bump to discard stale payloads
      // rather than migrate (the merge below Zod-validates what survives).
      version: 7,
      migrate: discardOlderVersions,
      merge: (persisted, current) => {
        if (persisted === undefined) return current;
        const parsed = sessionPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn(
            "[agentSessionStore] discarding corrupted scopeapp.agent-session:",
            parsed.error.issues,
          );
          return current;
        }
        return {
          ...current,
          ...parsed.data,
          draftSessionIds: new Set(parsed.data.draftSessionIds),
        };
      },
    },
  ),
);

// Prune draft refs for sessions no longer held open. Without this they grow
// unbounded (one stale entry per draft session abandoned before its first
// message), and a leftover draft id would make useAgentSession wrongly skip history hydration
// if that id were ever reopened. A live draft id is always present in
// openSessionIds (holdOpen is paired with selecting it), so "not open" ⇒ dead.
const unsubPruneSessionRefs = useAgentSessionStore.subscribe((state, prev) => {
  if (state.openSessionIds === prev.openSessionIds) return;
  const draftSessionIds = pruneDraftSessions(state);
  const open = new Set(state.openSessionIds);
  const freshDraftSessionIds = new Set(
    [...state.freshDraftSessionIds].filter((id) => open.has(id)),
  );
  if (draftSessionIds || freshDraftSessionIds.size !== state.freshDraftSessionIds.size) {
    useAgentSessionStore.setState({
      ...(draftSessionIds ? { draftSessionIds } : {}),
      freshDraftSessionIds,
    });
  }
});
disposeOnHmr(unsubPruneSessionRefs);
