import { nanoid } from "nanoid";
import { create } from "zustand";

// Composer data shapes. Declared here (the data owner) instead of in
// `components/chat/Composer.tsx` so the store doesn't have to import
// upward into the presentation layer. The icon field stays a `string`
// — components cast to their typed IconName union at render time.
export type ComposerMode = string;
export interface Attachment {
  /** Stable React-key id. Auto-assigned in `addAttachment` so callers
   *  don't have to manage it; supplying one explicitly (e.g. when
   *  hydrating from persistence) is allowed. */
  id: string;
  label: string;
  icon?: string;
}

interface ComposerState {
  value: string;
  mode: ComposerMode;
  attachments: Attachment[];
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
  clear: () => void;
  removeAttachment: (i: number) => void;
  addAttachment: (a: Omit<Attachment, "id"> & Partial<Pick<Attachment, "id">>) => void;
}

export const useComposerStore = create<ComposerState & ComposerActions>((set) => ({
  value: "",
  mode: "agent",
  attachments: [],
  provider: null,
  model: null,
  setValue: (value) => set({ value }),
  setMode: (mode) => set({ mode }),
  setModel: (provider, model) => set({ provider, model }),
  clear: () => set({ value: "" }),
  removeAttachment: (i) =>
    set((s) => ({ attachments: s.attachments.filter((_, idx) => idx !== i) })),
  addAttachment: (a) =>
    set((s) => ({ attachments: [...s.attachments, { id: a.id ?? nanoid(), ...a }] })),
}));
