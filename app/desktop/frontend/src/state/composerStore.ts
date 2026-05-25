import type { Attachment, ComposerMode } from "@/components/chat/Composer";
import { create } from "zustand";

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
