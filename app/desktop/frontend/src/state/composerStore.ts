import { nanoid } from "nanoid";
import { create } from "zustand";
import { fileToInputImage } from "@/lib/agent/composerInput";
import { notifyError } from "@/lib/notify";

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

interface ComposerState {
  value: string;
  /** Images staged for the next send (paste / drop / file-picker), inlined as
   *  image ContentBlocks. Cleared together with the text on submit. */
  images: ComposerImage[];
  /** Selected provider + model for the next run; both null = let the runtime
   *  pick its default. They're a pair (API §7.3) — set together by the
   *  composer's model selector (which knows each model's owning provider). */
  provider: string | null;
  model: string | null;
}

interface ComposerActions {
  setValue: (v: string) => void;
  setModel: (provider: string | null, model: string | null) => void;
  /** Wipe the text + every staged image (one call per successful submit). */
  clear: () => void;
  /** Stage one or more already-decoded images (ids auto-assigned). */
  addImages: (imgs: Omit<ComposerImage, "id">[]) => void;
  /** Read raw image Files (paste / drop / file-picker) to base64 and stage
   *  them — the File-ingesting counterpart to `addImages`. Fire-and-forget:
   *  the decode runs off the caller's path. Per-file tolerant (one unreadable
   *  file doesn't drop the whole batch; failures are surfaced via notify), and
   *  dropped entirely if the composer is cleared/submitted mid-decode so a
   *  late image can't leak into the next message. */
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
}

export const useComposerStore = create<ComposerState & ComposerActions>((set, get) => {
  // Bumped on every clear() (i.e. every submit). addImageFiles captures it when
  // its decode starts and drops the result if it advanced — so an image still
  // decoding when the user submits is discarded rather than leaking into the
  // NEXT message's composer.
  let stagingGen = 0;
  return {
    value: "",
    images: [],
    provider: null,
    model: null,
    setValue: (value) => set({ value }),
    setModel: (provider, model) => set({ provider, model }),
    clear: () => {
      stagingGen++;
      set({ value: "", images: [] });
    },
    addImages: (imgs) =>
      set((s) => ({ images: [...s.images, ...imgs.map((i) => ({ id: nanoid(), ...i }))] })),
    addImageFiles: (files) => {
      const gen = stagingGen;
      // allSettled, not all: one unreadable file must not discard the whole
      // batch, and the chain must never reject (there is no global
      // unhandledrejection handler). Stage every file that decoded; report the
      // rest. Drop everything if the composer was cleared mid-decode.
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
    removeImage: (id) => set((s) => ({ images: s.images.filter((i) => i.id !== id) })),
  };
});
