import { nanoid } from "nanoid";
import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/plugins/builtin/chat/composer/public/input";
import { countLines } from "@/plugins/builtin/chat/composer/public/largePaste";
import { disposeOnHmr } from "@/lib/hmr";
import { notifyError } from "@/lib/notify";
import { useSessionStore } from "@/state/sessionStore";
import type { ComposerImage, PastedText } from "../domain/draft";

// Store adapter for the composer draft read model.

/** One conversation's unsent composer content, kept PER SESSION so switching
 *  tabs never shows — or clobbers — another conversation's half-written
 *  message. Only `value` (text) is durable; staged images + pastes are
 *  transient (meant to be sent immediately, and heavy), so they're dropped on
 *  reload. */
interface Draft {
  value: string;
  images: ComposerImage[];
  pastes: PastedText[];
}
const emptyDraft = (): Draft => ({ value: "", images: [], pastes: [] });

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
  drafts: Record<string, Draft>;
  /** The session id `value`/`images` currently mirror. */
  activeSid: string;
  /** Per-session sent-message history (newest last, capped) for ↑/↓ recall.
   *  In-memory only — the transcript already survives reload; this is just the
   *  shell-style input ring for the current app session. */
  history: Record<string, string[]>;
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

// Persisted shape (trust boundary, validated on rehydrate): text-only drafts.
const persistSchema = z.object({
  drafts: z.record(z.string(), z.object({ value: z.string() })),
});

// Patch the active session's draft in one shot: apply `patch` over the current
// mirror, then write the result to BOTH the live mirror (value/images/pastes,
// which existing `s.value` selectors read) and its archive entry
// `drafts[activeSid]`. The "mirror === drafts[activeSid]" invariant lives HERE,
// so every mutation routes through it — passing only the fields it changes —
// instead of re-spelling the spread (and risking a desync).
function mirror(
  s: Pick<ComposerState, "activeSid" | "drafts" | "value" | "images" | "pastes">,
  patch: Partial<Draft>,
): Pick<ComposerState, "value" | "images" | "pastes" | "drafts"> {
  const draft: Draft = {
    value: patch.value ?? s.value,
    images: patch.images ?? s.images,
    pastes: patch.pastes ?? s.pastes,
  };
  return { ...draft, drafts: { ...s.drafts, [s.activeSid]: draft } };
}

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
        setValue: (value) => set((s) => ({ ...mirror(s, { value }), histIndex: -1 })),
        setModel: (provider, model) => set({ provider, model }),
        clear: () => {
          stagingGen++;
          set((s) => ({ ...mirror(s, emptyDraft()), histIndex: -1 }));
        },
        addImages: (imgs) =>
          set((s) =>
            mirror(s, { images: [...s.images, ...imgs.map((i) => ({ id: nanoid(), ...i }))] }),
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
        removeImage: (id) => set((s) => mirror(s, { images: s.images.filter((i) => i.id !== id) })),
        addPaste: (text) =>
          set((s) =>
            mirror(s, { pastes: [...s.pastes, { id: nanoid(), text, lines: countLines(text) }] }),
          ),
        removePaste: (id) => set((s) => mirror(s, { pastes: s.pastes.filter((p) => p.id !== id) })),
        loadSession: (sid) =>
          set((s) => {
            if (sid === s.activeSid) return s;
            const next = s.drafts[sid] ?? emptyDraft();
            return {
              activeSid: sid,
              value: next.value,
              images: next.images,
              pastes: next.pastes,
              histIndex: -1,
              histDraft: "",
            };
          }),
        pruneDrafts: (liveSids) =>
          set((s) => {
            const drafts: Record<string, Draft> = {};
            const history: Record<string, string[]> = {};
            const keep = (k: string) => k === "" || k === s.activeSid || liveSids.has(k);
            for (const k in s.drafts) if (keep(k)) drafts[k] = s.drafts[k]!;
            for (const k in s.history) if (keep(k)) history[k] = s.history[k]!;
            return { drafts, history };
          }),
        pushHistory: (text) =>
          set((s) => {
            const t = text.trim();
            if (!t) return s;
            const list = s.history[s.activeSid] ?? [];
            if (list[list.length - 1] === t) return { histIndex: -1 }; // dedupe consecutive
            return {
              history: { ...s.history, [s.activeSid]: [...list, t].slice(-HISTORY_CAP) },
              histIndex: -1,
            };
          }),
        historyPrev: () => {
          const s = get();
          const list = s.history[s.activeSid] ?? [];
          if (list.length === 0) return false;
          const idx = s.histIndex === -1 ? 0 : Math.min(s.histIndex + 1, list.length - 1);
          const value = list[list.length - 1 - idx]!;
          const histDraft = s.histIndex === -1 ? s.value : s.histDraft;
          set((st) => ({ ...mirror(st, { value }), histIndex: idx, histDraft }));
          return true;
        },
        historyNext: () => {
          const s = get();
          if (s.histIndex === -1) return false; // not navigating
          const list = s.history[s.activeSid] ?? [];
          const idx = s.histIndex - 1;
          // Past the newest entry → restore the draft that recall began from.
          const value = idx < 0 ? s.histDraft : list[list.length - 1 - idx]!;
          set((st) => ({ ...mirror(st, { value }), histIndex: idx < 0 ? -1 : idx }));
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
      partialize: (s) => ({
        drafts: Object.fromEntries(
          Object.entries(s.drafts).map(([k, d]) => [k, { value: d.value }]),
        ),
      }),
      merge: (persisted, current) => {
        const parsed = persistSchema.safeParse(persisted);
        if (!parsed.success) return current; // corrupt blob → empty drafts
        const drafts: Record<string, Draft> = {};
        for (const [k, d] of Object.entries(parsed.data.drafts))
          drafts[k] = { value: d.value, images: [], pastes: [] };
        return { ...current, drafts };
      },
    },
  ),
);

// Follow the active session: swap the mirrored draft on an activeSessionId
// change, and prune dead sessions' drafts on a tabIds change — the same
// lifecycle agentStore's view slices follow. Module-level subscription
// (app-lifetime), disposeOnHmr-guarded against dev hot-reload stacking.
const unsubDraftSync = useSessionStore.subscribe((state, prev) => {
  const composer = useComposerStore.getState();
  if (state.activeSessionId !== prev.activeSessionId) composer.loadSession(state.activeSessionId);
  if (state.tabIds !== prev.tabIds) composer.pruneDrafts(new Set(state.tabIds));
});
disposeOnHmr(unsubDraftSync);
// Seed the mirror from the restored active session on cold start — both stores
// rehydrate synchronously from localStorage, so activeSessionId is set here.
useComposerStore.getState().loadSession(useSessionStore.getState().activeSessionId);
