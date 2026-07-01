import { nanoid } from "nanoid";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/plugins/builtin/chat/composer/public/input";
import { countLines } from "@/plugins/builtin/chat/composer/public/largePaste";
import { notifyError } from "@/lib/notify";
import type { ComposerImage, PastedText } from "../domain/draft";
import {
  emptyComposerDraft,
  loadComposerDraft,
  mirrorComposerDraft,
  nextComposerHistory,
  parsePersistedComposerDrafts,
  persistedComposerDrafts,
  previousComposerHistory,
  pruneComposerArchives,
  pushComposerHistory,
  type ComposerDraftArchive,
  type ComposerHistoryArchive,
} from "../domain/draftArchive";

// Store adapter for the composer draft read model.

/** One conversation's unsent composer content, kept PER SESSION so switching
 *  tabs never shows — or clobbers — another conversation's half-written
 *  message. Only `value` (text) is durable; staged images + pastes are
 *  transient (meant to be sent immediately, and heavy), so they're dropped on
 *  reload. */

interface ComposerState {
  // value/images/pastes MIRROR the active session's draft so existing selectors
  // (`useComposerStore(s => s.value)`) keep working unchanged; `drafts` is the
  // per-session archive, swapped into the mirror by loadSession on the
  // active-session edge.
  value: string;
  images: ComposerImage[];
  pastes: PastedText[];
  // provider/model are a GLOBAL sticky preference — the picked model carries
  // across sessions (it's not per-conversation work) — so they stay top-level
  // and unmirrored. Not persisted: ModelPicker re-defaults to the first model.
  provider: string | null;
  model: string | null;
  /** Per-session draft archive, keyed by sessionId ("" = the no-session scratch
   *  draft on the welcome screen). */
  drafts: ComposerDraftArchive;
  /** The session id `value`/`images` currently mirror. */
  activeSid: string;
  /** Per-session sent-message history (newest last, capped) for ↑/↓ recall.
   *  In-memory only — the transcript already survives reload; this is just the
   *  shell-style input ring for the current app session. */
  history: ComposerHistoryArchive;
  /** Recall cursor into the active session's history: -1 = not navigating, 0 =
   *  most recent, 1 = next older … Reset whenever the user types or switches. */
  histIndex: number;
  /** The in-progress text saved when history recall began, so stepping back
   *  past the newest entry (↓) restores what was being typed. */
  histDraft: string;
}

interface ComposerActions {
  setValue: (v: string) => void;
  setModel: (provider: string | null, model: string | null) => void;
  /** Wipe the active session's text + every staged image (one call per submit). */
  clear: () => void;
  /** Stage one or more already-decoded images (ids auto-assigned). */
  addImages: (imgs: Omit<ComposerImage, "id">[]) => void;
  /** Read raw image Files (paste / drop / file-picker) to base64 and stage
   *  them — the File-ingesting counterpart to `addImages`. Fire-and-forget;
   *  per-file tolerant; dropped entirely if the composer is cleared/submitted
   *  OR the active session switches mid-decode, so a late image can't leak into
   *  the next message or another conversation's draft. */
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
  /** Stash a large pasted blob as a removable attachment chip, kept out of the
   *  textarea and re-inlined into the message on send (T2.3, large-paste). */
  addPaste: (text: string) => void;
  removePaste: (id: string) => void;
  /** Swap the mirrored draft to session `sid` — driven by the active-session
   *  subscription below. Mutations keep drafts[activeSid] current, so this only
   *  loads the target (nothing to archive). */
  loadSession: (sid: string) => void;
  /** Drop drafts whose session tab closed (mirrors agentStore's view prune) so
   *  the archive can't grow unbounded; the scratch ("") + active draft survive. */
  pruneDrafts: (liveSids: Set<string>) => void;
  /** Record a submitted message in the active session's recall history. */
  pushHistory: (text: string) => void;
  /** Step to the previous (older) history entry, saving the in-progress draft on
   *  the first step. Returns false when there's no history to recall (so the key
   *  falls through to normal cursor movement). */
  historyPrev: () => boolean;
  /** Step to the next (newer) entry, restoring the saved draft past the newest.
   *  Returns false when not currently navigating history. */
  historyNext: () => boolean;
}

// Per-session recall ring depth — enough to step back through a working
// session, bounded so it can't grow without limit.
const HISTORY_CAP = 50;

export const useComposerStore = create<ComposerState & ComposerActions>()(
  persist(
    (set, get) => {
      // Bumped on every clear() (i.e. every submit). addImageFiles captures it
      // when its decode starts and drops the result if it advanced — so an
      // image still decoding when the user submits is discarded rather than
      // leaking into the NEXT message.
      let stagingGen = 0;
      return {
        value: "",
        images: [],
        pastes: [],
        provider: null,
        model: null,
        drafts: {},
        activeSid: "",
        history: {},
        histIndex: -1,
        histDraft: "",
        // Editing (and clearing) exits history-recall mode (histIndex: -1).
        setValue: (value) => set((s) => ({ ...mirrorComposerDraft(s, { value }), histIndex: -1 })),
        setModel: (provider, model) => set({ provider, model }),
        clear: () => {
          stagingGen++;
          set((s) => ({ ...mirrorComposerDraft(s, emptyComposerDraft()), histIndex: -1 }));
        },
        addImages: (imgs) =>
          set((s) =>
            mirrorComposerDraft(s, {
              images: [...s.images, ...imgs.map((i) => ({ id: nanoid(), ...i }))],
            }),
          ),
        addImageFiles: (files) => {
          const gen = stagingGen;
          const sid = get().activeSid;
          // allSettled, not all: one unreadable file must not discard the whole
          // batch, and the chain must never reject (no global rejection handler).
          void Promise.allSettled(files.map(fileToInputImage)).then((results) => {
            // Discard if the composer was cleared/submitted (gen bumped) OR the
            // active session changed during decode — a late image must leak
            // neither into the next message NOR into another conversation's
            // draft (addImages writes the CURRENT activeSid's mirror).
            if (gen !== stagingGen || get().activeSid !== sid) return;
            const ok = results.flatMap((r) => (r.status === "fulfilled" ? [r.value] : []));
            if (ok.length > 0) get().addImages(ok);
            const failed = results.length - ok.length;
            if (failed > 0)
              notifyError(`Couldn't read ${failed} image${failed > 1 ? "s" : ""}`, {
                source: "composer",
              });
          });
        },
        removeImage: (id) =>
          set((s) => mirrorComposerDraft(s, { images: s.images.filter((i) => i.id !== id) })),
        addPaste: (text) =>
          set((s) =>
            mirrorComposerDraft(s, {
              pastes: [...s.pastes, { id: nanoid(), text, lines: countLines(text) }],
            }),
          ),
        removePaste: (id) =>
          set((s) => mirrorComposerDraft(s, { pastes: s.pastes.filter((p) => p.id !== id) })),
        loadSession: (sid) =>
          set((s) => {
            return loadComposerDraft(s, sid) ?? s;
          }),
        pruneDrafts: (liveSids) =>
          set((s) => {
            return pruneComposerArchives(s, liveSids);
          }),
        pushHistory: (text) =>
          set((s) => {
            return pushComposerHistory(s, text, HISTORY_CAP) ?? s;
          }),
        historyPrev: () => {
          const s = get();
          const next = previousComposerHistory(s);
          if (!next) return false;
          set((st) => ({
            ...mirrorComposerDraft(st, { value: next.value }),
            histIndex: next.histIndex,
            histDraft: next.histDraft,
          }));
          return true;
        },
        historyNext: () => {
          const s = get();
          const next = nextComposerHistory(s);
          if (!next) return false;
          set((st) => ({
            ...mirrorComposerDraft(st, { value: next.value }),
            histIndex: next.histIndex,
          }));
          return true;
        },
      };
    },
    {
      name: "lyra.composer",
      storage: createJSONStorage(() => localStorage),
      version: 1,
      // Persist text-only drafts. value/images/provider/model are NOT persisted:
      // value/images rehydrate from drafts via the cold-start loadSession below,
      // images are transient, and model re-defaults to the first available.
      partialize: (s) => ({ drafts: persistedComposerDrafts(s.drafts) }),
      merge: (persisted, current) => {
        const drafts = parsePersistedComposerDrafts(persisted);
        if (!drafts) return current;
        return { ...current, drafts };
      },
    },
  ),
);
