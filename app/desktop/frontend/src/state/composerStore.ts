import { create } from "zustand";

// Composer data shapes. Declared here (the data owner) instead of in
// `components/chat/Composer.tsx` so the store doesn't have to import
// upward into the presentation layer. The icon field stays a `string`
// — components cast to their typed IconName union at render time.
export type ComposerMode = string;
export interface Attachment {
  label: string;
  icon?: string;
}

interface ComposerState {
  value: string;
  mode: ComposerMode;
  attachments: Attachment[];
}

interface ComposerActions {
  setValue: (v: string) => void;
  setMode: (m: ComposerMode) => void;
  clear: () => void;
  removeAttachment: (i: number) => void;
  addAttachment: (a: Attachment) => void;
}

export const useComposerStore = create<ComposerState & ComposerActions>((set) => ({
  value: "",
  mode: "agent",
  attachments: [],
  setValue: (value) => set({ value }),
  setMode: (mode) => set({ mode }),
  clear: () => set({ value: "" }),
  removeAttachment: (i) =>
    set((s) => ({ attachments: s.attachments.filter((_, idx) => idx !== i) })),
  addAttachment: (a) => set((s) => ({ attachments: [...s.attachments, a] })),
}));
