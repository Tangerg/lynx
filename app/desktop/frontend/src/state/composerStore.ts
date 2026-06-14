import { nanoid } from "nanoid";
import { create } from "zustand";
import { fileToInputImage } from "@/lib/agent/composerInput";

// Composer data shapes. Declared here (the data owner) instead of in
// `components/chat/composer/Composer.tsx` so the store doesn't import upward
// into the presentation layer.
export type ComposerMode = string;

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
  mode: ComposerMode;
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
  setMode: (m: ComposerMode) => void;
  setModel: (provider: string | null, model: string | null) => void;
  /** Wipe the text + every staged image (one call per successful submit). */
  clear: () => void;
  /** Stage one or more already-decoded images (ids auto-assigned). */
  addImages: (imgs: Omit<ComposerImage, "id">[]) => void;
  /** Read raw image Files (paste / drop / file-picker) to base64 and stage
   *  them — the File-ingesting counterpart to `addImages`. Fire-and-forget:
   *  the FileReader decode runs off the caller's path. */
  addImageFiles: (files: File[]) => void;
  removeImage: (id: string) => void;
}

export const useComposerStore = create<ComposerState & ComposerActions>((set, get) => ({
  value: "",
  mode: "agent",
  images: [],
  provider: null,
  model: null,
  setValue: (value) => set({ value }),
  setMode: (mode) => set({ mode }),
  setModel: (provider, model) => set({ provider, model }),
  clear: () => set({ value: "", images: [] }),
  addImages: (imgs) =>
    set((s) => ({ images: [...s.images, ...imgs.map((i) => ({ id: nanoid(), ...i }))] })),
  addImageFiles: (files) =>
    void Promise.all(files.map(fileToInputImage)).then((imgs) => get().addImages(imgs)),
  removeImage: (id) => set((s) => ({ images: s.images.filter((i) => i.id !== id) })),
}));
