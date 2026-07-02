// Agent session UI state: open chat-session tabs, the active chat session,
// draft-session bookkeeping, and queued first messages.
//
// Persistence policy:
//   - Persisted: activeSessionId + tabIds (continuity across launches).
//   - Ephemeral: draftSessionIds, pendingMessages, selectionEpoch.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";
import type { AgentRunStartOptions } from "@/plugins/sdk/types";
import type { AgentInput } from "@/plugins/builtin/agent/domain/input";
import {
  closeSessionTab,
  pruneSessionHandoffs,
  reconcileSessionTabs,
  selectSessionTab,
} from "../application/session/sessionSelectionModel";

// localStorage payload schema. Mirrors `partialize` below — only the
// two continuity fields. Anything else in storage is dropped on
// rehydrate; a malformed entry falls back to defaults instead of
// crashing the boot.
const sessionPersistSchema = z.object({
  activeSessionId: z.string(),
  tabIds: z.array(z.string()),
});

export interface PendingAgentMessage {
  input: AgentInput;
  runOptions: AgentRunStartOptions;
}

interface AgentSessionState {
  activeSessionId: string;
  selectionEpoch: number;
  /** The set of sessions currently held open. selectTab opens (adds the id);
   *  closeTab / useDeleteSession close (removes it). Load-bearing plumbing
   *  despite the tab-strip UI being gone (Step 2a): it's the "live session"
   *  signal that drives per-session pruning across stores — agentStore drops
   *  view state, composerStore drops drafts, and this store's own subscription
   *  drops draft + pending-message refs for ids no longer in the set. */
  tabIds: string[];

  /**
   * Draft sessions — real backend sessions (created up front so they can
   * receive a run) that haven't had their first message yet. Hidden from
   * the Work Index until they "graduate" (first send), so a fresh
   * "New" doesn't litter the list with empties. Ephemeral (not persisted).
   */
  draftSessionIds: Set<string>;
  /**
   * First message queued for a freshly-created session, keyed by id. When
   * the user types on the welcome screen (no active session), we create a
   * draft and stash the input here (text + any inlined images); the chat
   * remounts on the new id and flushes it. Ephemeral.
   */
  pendingMessages: Record<string, PendingAgentMessage>;
}

interface AgentSessionActions {
  selectTab: (id: string) => void;
  closeTab: (id: string) => void;

  /** Mark a session as a hidden draft (just created, no message yet). */
  markDraft: (id: string) => void;
  /** Promote a draft to a real session (first message sent). Idempotent. */
  graduateDraft: (id: string) => void;
  /** Reconcile persisted tabs against the backend's live session ids (boot):
   *  drop tab / active refs to sessions the runtime no longer has — deleted
   *  elsewhere, or the whole db reset (`make fresh`) — so a persisted ghost id
   *  can't strand the user on a dead session that runs.start rejects with
   *  session_not_found. Not-yet-graduated drafts (absent from sessions.list by
   *  design) are kept. */
  reconcileTabs: (liveIds: string[]) => void;
  /** Queue the first message input for a session id. */
  setPendingMessage: (id: string, message: PendingAgentMessage) => void;
  /** Read + clear the queued first message input for a session id. */
  takePendingMessage: (id: string) => PendingAgentMessage | undefined;
}

export const useAgentSessionStore = create<AgentSessionState & AgentSessionActions>()(
  persist(
    (set, get) => ({
      // No demo fixtures — open tabs + active session start empty and are
      // driven by the real backend's sessions.list (the sidebar) + user
      // clicks. Ghost ids would make the chat try to load/run a session the
      // runtime doesn't have (session_not_found on boot).
      activeSessionId: "",
      selectionEpoch: 0,
      tabIds: [],
      draftSessionIds: new Set<string>(),
      pendingMessages: {},

      selectTab: (id) => {
        const { activeSessionId, tabIds, selectionEpoch } = get();
        set(selectSessionTab({ activeSessionId, tabIds, selectionEpoch }, id));
      },
      closeTab: (id) => {
        const { tabIds, activeSessionId } = get();
        set(closeSessionTab({ activeSessionId, tabIds }, id));
      },
      markDraft: (id) => set({ draftSessionIds: new Set(get().draftSessionIds).add(id) }),
      graduateDraft: (id) => {
        const drafts = get().draftSessionIds;
        if (!drafts.has(id)) return;
        const next = new Set(drafts);
        next.delete(id);
        set({ draftSessionIds: next });
      },
      reconcileTabs: (liveIds) => {
        const { tabIds, activeSessionId, draftSessionIds } = get();
        const next = reconcileSessionTabs({ activeSessionId, tabIds, draftSessionIds }, liveIds);
        if (next) set(next);
      },
      setPendingMessage: (id, message) =>
        set({ pendingMessages: { ...get().pendingMessages, [id]: message } }),
      takePendingMessage: (id) => {
        const { pendingMessages } = get();
        const input = pendingMessages[id];
        if (input === undefined) return undefined;
        const next = { ...pendingMessages };
        delete next[id];
        set({ pendingMessages: next });
        return input;
      },
    }),
    {
      name: "lyra.agent-session",
      storage: createJSONStorage(() => localStorage),
      // Persist only the continuity fields. Draft and pending-message refs are
      // ephemeral because they point at in-memory first-run handoff state.
      partialize: (s) => ({
        activeSessionId: s.activeSessionId,
        tabIds: s.tabIds,
      }),
      // Persisted shape is dev-phase only; bump to discard stale payloads
      // rather than migrate (the merge below Zod-validates what survives).
      version: 4,
      merge: (persisted, current) => {
        const parsed = sessionPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn(
            "[agentSessionStore] discarding corrupted lyra.agent-session:",
            parsed.error.issues,
          );
          return current;
        }
        return { ...current, ...parsed.data };
      },
    },
  ),
);

// Prune draft + pending-message refs for sessions whose tab has closed.
// Both maps are keyed by session id; without this they grow unbounded (one
// stale entry per draft tab abandoned before its first message), and a
// leftover draft id would make useAgentSession wrongly skip history hydration
// if that id were ever reopened. A live draft id is always present in tabIds
// (markDraft is paired with selectTab), so "not in tabIds" ⇒ dead. One
// subscription catches every removal path (closeTab, close-* helpers,
// useDeleteSession → closeTab).
const unsubPruneSessionRefs = useAgentSessionStore.subscribe((state, prev) => {
  if (state.tabIds === prev.tabIds) return;
  const next = pruneSessionHandoffs(state);
  if (next) useAgentSessionStore.setState(next);
});
disposeOnHmr(unsubPruneSessionRefs);
