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
import type { ContentBlock } from "@/rpc";

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

interface SessionState {
  activeSessionId: string;
  /** The set of sessions currently held open. selectTab opens (adds the id);
   *  closeTab / useDeleteSession close (removes it). Load-bearing plumbing
   *  despite the tab-strip UI being gone (Step 2a): it's the "live session"
   *  signal that drives per-session pruning across stores — agentStore drops
   *  view state, composerStore drops drafts, and this store's own subscription
   *  drops draft + pending-message refs for ids no longer in the set. */
  tabIds: string[];

  /** Workspace views the user has opened in the main pane (openMainView adds,
   *  closeMainView removes). The tab-strip UI is gone (Step 2a), so nothing
   *  renders this list directly — it survives as the bookkeeping that lets
   *  closeMainView fall back to the last remaining view when the active one
   *  closes, and keeps openMainView / openMainViewBeside / promoteSplitToTab
   *  idempotent (re-opening an existing view focuses instead of duplicating). */
  mainViewTabs: MainViewTab[];
  activeMainView: string | null;
  /**
   * One-shot deep-link target for the Settings view: the pane id to open it
   * at (e.g. "providers" from the keyless first-run onboarding). SettingsPage
   * consumes it once on mount and clears it, so a later manual open starts at
   * the first pane. null = open at the first pane. Ephemeral (not persisted).
   */
  settingsPane: string | null;
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
   * draft and stash the input here (text + any inlined images); the chat
   * remounts on the new id and flushes it. Ephemeral.
   */
  pendingMessages: Record<string, ContentBlock[]>;

  activeFile: string;
  /** The file + 1-based line the FileView shows, set by a clickable file:line
   *  reference (0 = no specific line). Null = nothing open. Session-scoped. */
  fileViewer: { path: string; line: number } | null;
  selectedToolId: string;
  expandedToolIds: Set<string>;
}

interface SessionActions {
  selectTab: (id: string) => void;
  closeTab: (id: string) => void;

  /** Mark a session as a hidden draft (just created, no message yet). */
  markDraft: (id: string) => void;
  /** Promote a draft to a real session (first message sent). Idempotent. */
  graduateDraft: (id: string) => void;
  /** Queue the first message input for a session id. */
  setPendingMessage: (id: string, input: ContentBlock[]) => void;
  /** Read + clear the queued first message input for a session id. */
  takePendingMessage: (id: string) => ContentBlock[] | undefined;

  /** Set the one-shot pane the Settings view opens at (null = first pane). */
  setSettingsPane: (pane: string | null) => void;
  /** Add (if absent) and focus a workspace view in the chat-area tab strip. */
  openMainView: (tab: MainViewTab) => void;
  /** Remove a workspace view tab; falls back to chat if it was active. */
  closeMainView: (id: string) => void;
  /** Clear the workspace view focus so the chat session takes over again. */
  selectChat: () => void;
  /** Open (or focus) a splittable view BESIDE the chat stream (resizable split). */
  openMainViewBeside: (tab: MainViewTab) => void;
  /** Close the side-by-side split pane (chat returns to full width). */
  closeSplit: () => void;
  /** Promote the split (beside) view to a full-width main tab. Mutually
   *  exclusive with the split (clears it). No-op when no split is open. */
  promoteSplitToTab: () => void;

  setActiveFile: (path: string) => void;
  /** Open the file viewer on `path` (optionally at a 1-based `line`) and promote
   *  the FileView tab — the target of a clickable file:line reference. */
  openFileViewer: (path: string, line?: number) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
}

// State scoped to the active chat session — the inspected tool (+ its expanded
// rows), the file the diff view is focused on, and the beside-split (which
// shows THIS session's tool detail and collapses the nav rail). It all belongs
// to the session you're looking at, so every action that switches to a
// DIFFERENT session clears it. Returned as a patch applied in the SAME set() as
// the activeSessionId change — clearing it via a post-hoc subscription would
// flash the old session's split/file for a frame. activeMainView is deliberately
// excluded: promoted view tabs are global, cleared by selectTab on its own rule.
function clearSessionScopedState() {
  return {
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
    splitViewId: null,
  };
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
      settingsPane: null,
      splitViewId: null,
      draftSessionIds: new Set<string>(),
      pendingMessages: {},
      activeFile: "",
      fileViewer: null,
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
          // Switching to a different session drops everything scoped to the one
          // you left. Cleared HERE (not per call site): sidebar rows, Cmd+1..9,
          // ⌘N and the tab strip all funnel through selectTab.
          ...(id === activeSessionId ? {} : clearSessionScopedState()),
        });
      },
      closeTab: (id) => {
        const { tabIds, activeSessionId } = get();
        const idx = tabIds.indexOf(id);
        const next = tabIds.filter((x) => x !== id);
        const leavingActive = id === activeSessionId;
        set({
          tabIds: next,
          // Closing the active tab reselects the ADJACENT tab — the one that
          // shifts into this slot (`next[idx]`), or the new last tab when the
          // rightmost closed (`next.at(-1)`) — falling back to "" (welcome
          // screen) when nothing remains. Never the always-leftmost `next[0]`
          // (that yanks focus across the strip), and never a closed/deleted id.
          activeSessionId: leavingActive ? (next[idx] ?? next.at(-1) ?? "") : activeSessionId,
          // Leaving the active session drops its inspector / file / split so
          // they don't bleed onto the neighbour (or the welcome screen).
          ...(leavingActive ? clearSessionScopedState() : {}),
        });
      },
      markDraft: (id) => set({ draftSessionIds: new Set(get().draftSessionIds).add(id) }),
      graduateDraft: (id) => {
        const drafts = get().draftSessionIds;
        if (!drafts.has(id)) return;
        const next = new Set(drafts);
        next.delete(id);
        set({ draftSessionIds: next });
      },
      setPendingMessage: (id, input) =>
        set({ pendingMessages: { ...get().pendingMessages, [id]: input } }),
      takePendingMessage: (id) => {
        const { pendingMessages } = get();
        const input = pendingMessages[id];
        if (input === undefined) return undefined;
        const next = { ...pendingMessages };
        delete next[id];
        set({ pendingMessages: next });
        return input;
      },

      setSettingsPane: (pane) => set({ settingsPane: pane }),
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
      promoteSplitToTab: () => {
        const { splitViewId, mainViewTabs } = get();
        const tab = splitViewId ? mainViewTabs.find((t) => t.id === splitViewId) : undefined;
        if (tab) get().openMainView(tab); // openMainView clears splitViewId (mutually exclusive)
      },
      closeMainView: (id) => {
        const cur = get().mainViewTabs;
        const next = cur.filter((t) => t.id !== id);
        const activeMainView =
          get().activeMainView === id ? (next.at(-1)?.id ?? null) : get().activeMainView;
        const splitViewId = get().splitViewId === id ? null : get().splitViewId;
        set({ mainViewTabs: next, activeMainView, splitViewId });
      },
      selectChat: () => set({ activeMainView: null }),

      setActiveFile: (path) => set({ activeFile: path }),
      openFileViewer: (path, line) => {
        get().openMainView({ id: "file", title: "workspace.view.title.file", icon: "filetext" });
        set({ fileViewer: { path, line: line ?? 0 } });
      },
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
      version: 3,
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
