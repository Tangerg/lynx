// Bits shared by the built-in tool previews (index.tsx + specialised.tsx).

// Shared container shape for every inline tool preview. The wrapper lives
// inside a bg-surface card (the expanded activity row), so it uses no
// additional background — just padding and typography.
export const PREVIEW_WRAP =
  "max-h-60 overflow-y-auto px-0 pt-1 pb-0 font-mono text-[12px] leading-[1.55] text-fg-muted";
