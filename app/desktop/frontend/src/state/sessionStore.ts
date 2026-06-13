// Session-scoped UI state: open chat-session tabs, promoted workspace
// view tabs, current file the user is looking at, and per-session tool
// inspector state (selected / expanded ids).
//
// Persistence policy:
//   - Persisted: activeSessionId + tabIds (continuity across launches).
//   - Ephemeral: mainViewTabs, activeMainView, activeFile,
//     selectedToolId, expandedToolIds — all reference data that may not
//     exist or may have changed on next boot.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";

// localStorage payload schema. Mirrors `partialize` below — only the
// two continuity fields. Anything else in storage is dropped on
// rehydrate; a malformed entry falls back to defaults instead of
// crashing the boot.
const sessionPersistSchema = z.object({
  activeSessionId: z.string(),
  tabIds: z.array(z.string()),
});

interface MainViewTab {
  id: string;
  title: string;
  icon?: string;
}

/**
 * Discriminator for the two tab kinds the chat header juggles. Chat
 * tabs sit on the left of the unified strip, view tabs (workspace
 * views — including the Settings pane opened via host.openMainView)
 * sit on the right.
 */
export type HeaderTabKind = "chat" | "view";

/**
 * Close closures already bound to a specific right-clicked tab.
 * Lives next to the store because each closure composes several
 * underlying store actions (chat + view tabs together).
 */
export interface HeaderTabCloseActions {
  onCloseOthers: () => void;
  onCloseLeft: () => void;
  onCloseRight: () => void;
  onCloseAll: () => void;
}

interface SessionState {
  activeSessionId: string;
  tabIds: string[];

  /**
   * Heterogeneous chat-area tabs.
   *
   * Each entry is a workspace view the user "promoted" into the main
   * pane to read at full width. When `activeMainView` is set, the chat
   * panel renders that view's component instead of the message stream.
   * Selecting a chat session tab clears `activeMainView`.
   */
  mainViewTabs: MainViewTab[];
  activeMainView: string | null;
  /**
   * A splittable workspace view shown BESIDE the chat stream (resizable),
   * not replacing it. Mutually exclusive with `activeMainView` (opening one
   * clears the other). null = no side pane, chat is full-width.
   */
  splitViewId: string | null;

  /**
   * Draft sessions — real backend sessions (created up front so they can
   * receive a run) that haven't had their first message yet. Hidden from
   * the sidebar list until they "graduate" (first send), so a fresh
   * "New" doesn't litter the list with empties. Ephemeral (not persisted).
   */
  draftSessionIds: Set<string>;
  /**
   * First message queued for a freshly-created session, keyed by id. When
   * the user types on the welcome screen (no active session), we create a
   * draft and stash the text here; the chat remounts on the new id and
   * flushes it. Ephemeral.
   */
  pendingMessages: Record<string, string>;

  activeFile: string;
  selectedToolId: string;
  expandedToolIds: Set<string>;
}

interface SessionActions {
  selectTab: (id: string) => void;
  closeTab: (id: string) => void;
  openTab: (id: string) => void;

  /** Mark a session as a hidden draft (just created, no message yet). */
  markDraft: (id: string) => void;
  /** Promote a draft to a real session (first message sent). Idempotent. */
  graduateDraft: (id: string) => void;
  /** Queue the first message for a session id. */
  setPendingMessage: (id: string, text: string) => void;
  /** Read + clear the queued first message for a session id. */
  takePendingMessage: (id: string) => string | undefined;

  /** Close every chat tab except `id`. */
  closeOtherTabs: (id: string) => void;
  /** Close every chat tab whose position precedes `id` in `tabIds`. */
  closeTabsLeftOf: (id: string) => void;
  /** Close every chat tab whose position follows `id` in `tabIds`. */
  closeTabsRightOf: (id: string) => void;
  /** Close every chat tab. */
  closeAllTabs: () => void;

  /** Add (if absent) and focus a workspace view in the chat-area tab strip. */
  openMainView: (tab: MainViewTab) => void;
  /** Remove a workspace view tab; falls back to chat if it was active. */
  closeMainView: (id: string) => void;
  /** Focus a workspace view tab without opening a new one. */
  selectMainView: (id: string) => void;
  /** Clear the workspace view focus so the chat session takes over again. */
  selectChat: () => void;
  /** Open (or focus) a splittable view BESIDE the chat stream (resizable split). */
  openMainViewBeside: (tab: MainViewTab) => void;
  /** Close the side-by-side split pane (chat returns to full width). */
  closeSplit: () => void;

  /** Close every workspace-view tab except `id`. */
  closeOtherMainViews: (id: string) => void;
  /** Close every workspace-view tab whose position precedes `id`. */
  closeMainViewsLeftOf: (id: string) => void;
  /** Close every workspace-view tab whose position follows `id`. */
  closeMainViewsRightOf: (id: string) => void;
  /** Close every workspace-view tab. */
  closeAllMainViews: () => void;

  setActiveFile: (path: string) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
}

export const useSessionStore = create<SessionState & SessionActions>()(
  persist(
    (set, get) => ({
      // No demo fixtures — open tabs + active session start empty and are
      // driven by the real backend's sessions.list (the sidebar) + user
      // clicks. Ghost ids would make the chat try to load/run a session the
      // runtime doesn't have (session_not_found on boot).
      activeSessionId: "",
      tabIds: [],
      mainViewTabs: [],
      activeMainView: null,
      splitViewId: null,
      draftSessionIds: new Set<string>(),
      pendingMessages: {},
      activeFile: "",
      selectedToolId: "",
      expandedToolIds: new Set<string>(),

      selectTab: (id) => {
        const { tabIds, activeSessionId } = get();
        set({
          activeSessionId: id,
          tabIds: tabIds.includes(id) ? tabIds : [...tabIds, id],
          // Selecting a chat session always returns the main pane to the
          // message stream — a promoted workspace view (Settings/Diagnostics
          // …) must not stay focused, or the click visibly "does nothing".
          // Cleared HERE (not per call site): sidebar rows, Cmd+1..9, ⌘N and
          // the tab strip all funnel through selectTab.
          activeMainView: null,
          // Tool-inspector + file focus are session-scoped. Switching to a
          // different session must not carry A's selection/expansion into B
          // (the ids wouldn't match B's items, and expandedToolIds would
          // otherwise accrete every session's ids forever).
          ...(id === activeSessionId
            ? {}
            : { activeFile: "", selectedToolId: "", expandedToolIds: new Set<string>() }),
        });
      },
      closeTab: (id) => {
        const { tabIds, activeSessionId } = get();
        const next = tabIds.filter((x) => x !== id);
        set({
          tabIds: next,
          // Closing the active tab reselects its neighbour, or falls back to
          // "" (welcome screen) when nothing remains — never leave
          // activeSessionId pointing at a closed/deleted session.
          activeSessionId: id === activeSessionId ? (next[0] ?? "") : activeSessionId,
        });
      },
      openTab: (id) => {
        const { tabIds } = get();
        if (!tabIds.includes(id)) set({ tabIds: [...tabIds, id] });
      },

      markDraft: (id) => set({ draftSessionIds: new Set(get().draftSessionIds).add(id) }),
      graduateDraft: (id) => {
        const drafts = get().draftSessionIds;
        if (!drafts.has(id)) return;
        const next = new Set(drafts);
        next.delete(id);
        set({ draftSessionIds: next });
      },
      setPendingMessage: (id, text) =>
        set({ pendingMessages: { ...get().pendingMessages, [id]: text } }),
      takePendingMessage: (id) => {
        const { pendingMessages } = get();
        const text = pendingMessages[id];
        if (text === undefined) return undefined;
        const next = { ...pendingMessages };
        delete next[id];
        set({ pendingMessages: next });
        return text;
      },

      // Multi-tab close helpers — all preserve `activeSessionId`
      // when the active tab survives, otherwise fall back to the
      // leftmost remaining tab (or empty string when nothing is
      // left, mirroring the original closeTab semantics).
      closeOtherTabs: (id) => {
        const { tabIds } = get();
        if (!tabIds.includes(id)) return;
        set({ tabIds: [id], activeSessionId: id });
      },
      closeTabsLeftOf: (id) => {
        const { tabIds, activeSessionId } = get();
        const idx = tabIds.indexOf(id);
        if (idx <= 0) return;
        const next = tabIds.slice(idx);
        set({
          tabIds: next,
          activeSessionId: next.includes(activeSessionId) ? activeSessionId : id,
        });
      },
      closeTabsRightOf: (id) => {
        const { tabIds, activeSessionId } = get();
        const idx = tabIds.indexOf(id);
        if (idx === -1 || idx === tabIds.length - 1) return;
        const next = tabIds.slice(0, idx + 1);
        set({
          tabIds: next,
          activeSessionId: next.includes(activeSessionId) ? activeSessionId : id,
        });
      },
      closeAllTabs: () => {
        set({ tabIds: [], activeSessionId: "" });
      },

      openMainView: (tab) => {
        const cur = get().mainViewTabs;
        const exists = cur.some((t) => t.id === tab.id);
        set({
          mainViewTabs: exists ? cur : [...cur, tab],
          activeMainView: tab.id,
          splitViewId: null, // a full view + a side pane are mutually exclusive
        });
      },
      openMainViewBeside: (tab) => {
        const cur = get().mainViewTabs;
        const exists = cur.some((t) => t.id === tab.id);
        set({
          mainViewTabs: exists ? cur : [...cur, tab],
          splitViewId: tab.id,
          activeMainView: null, // the chat stream owns the other half
        });
      },
      closeSplit: () => set({ splitViewId: null }),
      closeMainView: (id) => {
        const cur = get().mainViewTabs;
        const next = cur.filter((t) => t.id !== id);
        const activeMainView =
          get().activeMainView === id ? (next.at(-1)?.id ?? null) : get().activeMainView;
        const splitViewId = get().splitViewId === id ? null : get().splitViewId;
        set({ mainViewTabs: next, activeMainView, splitViewId });
      },
      selectMainView: (id) => set({ activeMainView: id, splitViewId: null }),
      selectChat: () => set({ activeMainView: null }),

      // Same shape as the chat-tab close helpers, scoped to the
      // workspace-view strip.
      closeOtherMainViews: (id) => {
        const cur = get().mainViewTabs;
        const target = cur.find((t) => t.id === id);
        if (!target) return;
        set({ mainViewTabs: [target], activeMainView: id });
      },
      closeMainViewsLeftOf: (id) => {
        const { mainViewTabs, activeMainView } = get();
        const idx = mainViewTabs.findIndex((t) => t.id === id);
        if (idx <= 0) return;
        const next = mainViewTabs.slice(idx);
        set({
          mainViewTabs: next,
          activeMainView:
            activeMainView && next.some((t) => t.id === activeMainView) ? activeMainView : id,
        });
      },
      closeMainViewsRightOf: (id) => {
        const { mainViewTabs, activeMainView } = get();
        const idx = mainViewTabs.findIndex((t) => t.id === id);
        if (idx === -1 || idx === mainViewTabs.length - 1) return;
        const next = mainViewTabs.slice(0, idx + 1);
        set({
          mainViewTabs: next,
          activeMainView:
            activeMainView && next.some((t) => t.id === activeMainView) ? activeMainView : id,
        });
      },
      closeAllMainViews: () => {
        set({ mainViewTabs: [], activeMainView: null, splitViewId: null });
      },

      setActiveFile: (path) => set({ activeFile: path }),
      setSelectedToolId: (id) => set({ selectedToolId: id }),
      toggleExpandedTool: (id) => {
        const next = new Set(get().expandedToolIds);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        set({ expandedToolIds: next });
      },
    }),
    {
      name: "lyra.session",
      storage: createJSONStorage(() => localStorage),
      // Persist only the continuity fields. Tool inspector + main views
      // + activeFile are intentionally session-scoped (the underlying
      // data may not exist on next boot).
      partialize: (s) => ({
        activeSessionId: s.activeSessionId,
        tabIds: s.tabIds,
      }),
      // Persisted shape is dev-phase only; bump to discard stale payloads
      // rather than migrate (the merge below Zod-validates what survives).
      version: 2,
      merge: (persisted, current) => {
        const parsed = sessionPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn("[sessionStore] discarding corrupted lyra.session:", parsed.error.issues);
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
const unsubPruneSessionRefs = useSessionStore.subscribe((state, prev) => {
  if (state.tabIds === prev.tabIds) return;
  const live = new Set(state.tabIds);
  const draftStale = [...state.draftSessionIds].some((id) => !live.has(id));
  const pendingStale = Object.keys(state.pendingMessages).some((id) => !live.has(id));
  if (!draftStale && !pendingStale) return;
  useSessionStore.setState({
    draftSessionIds: new Set([...state.draftSessionIds].filter((id) => live.has(id))),
    pendingMessages: Object.fromEntries(
      Object.entries(state.pendingMessages).filter(([id]) => live.has(id)),
    ),
  });
});
disposeOnHmr(unsubPruneSessionRefs);

/**
 * Compose a tab kind + id into close-action closures with unified-
 * strip semantics. Chat tabs render before view tabs in the header,
 * so:
 *
 *   - Left-of a view tab includes EVERY chat tab plus preceding views.
 *   - Right-of a chat tab includes the trailing chat tabs AND every
 *     view tab.
 *   - "Close Others" / "Close All" wipe both kinds regardless of
 *     which kind was clicked.
 *
 * Lives in the store layer (not in PanelHeader) so the cross-kind
 * sequencing is unit-testable without rendering.
 */
export function headerTabCloseActionsFor(kind: HeaderTabKind, id: string): HeaderTabCloseActions {
  const s = () => useSessionStore.getState();
  const closeAll = () => {
    s().closeAllTabs();
    s().closeAllMainViews();
  };
  if (kind === "chat") {
    return {
      onCloseOthers: () => {
        s().closeOtherTabs(id);
        s().closeAllMainViews();
      },
      onCloseLeft: () => s().closeTabsLeftOf(id),
      onCloseRight: () => {
        s().closeTabsRightOf(id);
        s().closeAllMainViews();
      },
      onCloseAll: closeAll,
    };
  }
  return {
    onCloseOthers: () => {
      s().closeAllTabs();
      s().closeOtherMainViews(id);
    },
    onCloseLeft: () => {
      s().closeAllTabs();
      s().closeMainViewsLeftOf(id);
    },
    onCloseRight: () => s().closeMainViewsRightOf(id),
    onCloseAll: closeAll,
  };
}
