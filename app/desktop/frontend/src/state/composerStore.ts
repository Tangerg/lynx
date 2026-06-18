import { nanoid } from "nanoid";
import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { fileToInputImage } from "@/lib/agent/composerInput";
import { disposeOnHmr } from "@/lib/hmr";
import { notifyError } from "@/lib/notify";
import { useSessionStore } from "./sessionStore";

// Composer data shapes. Declared here (the data owner) instead of in
// `components/chat/composer/Composer.tsx` so the store doesn't import upward
// into the presentation layer.

/** One image staged in the composer, ready to inline on send. `data` is raw
 *  base64 (NO "data:" prefix) — the wire form of an image ContentBlock
 *  (MULTIMODAL_IMAGE_INPUT, API.md §4.3). */
export interface ComposerImage {
  /** Stable React-key id, auto-assigned in `addImages`. */
  id: string;
  mime: string;
  data: string;
  /** Original file name — thumbnail tooltip / alt text. */
  name?: string;
}

/** One conversation's unsent composer content, kept PER SESSION so switching
 *  tabs never shows — or clobbers — another conversation's half-written
 *  message. Only `value` (text) is durable; staged images are transient (meant
 *  to be sent immediately, and heavy as base64), so they're dropped on reload. */
interface Draft {
  value: string;
  images: ComposerImage[];
}
const emptyDraft = (): Draft => ({ value: "", images: [] });

interface ComposerState {
  // value/images MIRROR the active session's draft so existing selectors
  // (`useComposerStore(s => s.value)`) keep working unchanged; `drafts` is the
  // per-session archive, swapped into the mirror by loadSession on the
  // active-session edge.
  value: string;
  images: ComposerImage[];
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
   *  mid-decode so a late image can't leak into the next message. */
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
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
        provider: null,
        model: null,
        drafts: {},
        activeSid: "",
        history: {},
        histIndex: -1,
        histDraft: "",
        setValue: (value) =>
          set((s) => ({
            value,
            histIndex: -1, // editing leaves history-recall mode
            drafts: { ...s.drafts, [s.activeSid]: { value, images: s.images } },
          })),
        setModel: (provider, model) => set({ provider, model }),
        clear: () => {
          stagingGen++;
          set((s) => ({
            value: "",
            images: [],
            histIndex: -1,
            drafts: { ...s.drafts, [s.activeSid]: emptyDraft() },
          }));
        },
        addImages: (imgs) =>
          set((s) => {
            const images = [...s.images, ...imgs.map((i) => ({ id: nanoid(), ...i }))];
            return { images, drafts: { ...s.drafts, [s.activeSid]: { value: s.value, images } } };
          }),
        addImageFiles: (files) => {
          const gen = stagingGen;
          // allSettled, not all: one unreadable file must not discard the whole
          // batch, and the chain must never reject (no global rejection handler).
          void Promise.allSettled(files.map(fileToInputImage)).then((results) => {
            if (gen !== stagingGen) return; // cleared / submitted while decoding
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
          set((s) => {
            const images = s.images.filter((i) => i.id !== id);
            return { images, drafts: { ...s.drafts, [s.activeSid]: { value: s.value, images } } };
          }),
        loadSession: (sid) =>
          set((s) => {
            if (sid === s.activeSid) return s;
            const next = s.drafts[sid] ?? emptyDraft();
            return {
              activeSid: sid,
              value: next.value,
              images: next.images,
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
          set((st) => ({
            value,
            histIndex: idx,
            histDraft,
            drafts: { ...st.drafts, [st.activeSid]: { value, images: st.images } },
          }));
          return true;
        },
        historyNext: () => {
          const s = get();
          if (s.histIndex === -1) return false; // not navigating
          const list = s.history[s.activeSid] ?? [];
          const idx = s.histIndex - 1;
          // Past the newest entry → restore the draft that recall began from.
          const value = idx < 0 ? s.histDraft : list[list.length - 1 - idx]!;
          set((st) => ({
            value,
            histIndex: idx < 0 ? -1 : idx,
            drafts: { ...st.drafts, [st.activeSid]: { value, images: st.images } },
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
          drafts[k] = { value: d.value, images: [] };
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
