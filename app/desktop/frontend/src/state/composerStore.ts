import { create } from "zustand";
import type { Attachment, ComposerMode } from "@/components/chat/Composer";

type ComposerState = {
  value: string;
  mode: ComposerMode;
  attachments: Attachment[];
};

type ComposerActions = {
  setValue: (v: string) => void;
  setMode: (m: ComposerMode) => void;
  clear: () => void;
  removeAttachment: (i: number) => void;
  addAttachment: (a: Attachment) => void;
};

export const useComposerStore = create<ComposerState & ComposerActions>((set) => ({
  value: "",
  mode: "agent",
  attachments: [],
  setValue: (value) => set({ value }),
  setMode: (mode) => set({ mode }),
  clear: () => set({ value: "" }),
  removeAttachment: (i) => set((s) => ({ attachments: s.attachments.filter((_, idx) => idx !== i) })),
  addAttachment: (a) => set((s) => ({ attachments: [...s.attachments, a] })),
}));

// Default sample chips used to live here as the initial `attachments`
// array. They moved into `lyra.builtin.sample-attachments` (a plugin-
// contributed attachment source); the store itself now starts empty,
// reflecting "the user hasn't attached anything yet".
