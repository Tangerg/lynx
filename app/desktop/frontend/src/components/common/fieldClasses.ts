// Shared chrome for inline text fields — the small path/rename inputs
// (relocate banner, add-project popover) and the memory editor textarea.
// Canvas-sunken, mono; size, padding, and ink tone go at the call site via
// cn(). Focus is a quiet neutral border ring — no accent glow (the redesign
// drops the cheap bright focus edges).
export const FIELD_CLASSES =
  "rounded-md bg-canvas font-mono text-[12px] outline-none focus:ring-1 focus:ring-line-soft";

// Focus treatment for the bordered settings-form text inputs (Providers / MCP /
// Connection panes): the border quietly strengthens on focus — no accent halo
// (that bright ring was one of the "cheap" tells). Compose onto the input's own
// size/padding with cn().
export const INPUT_FOCUS_RING = "focus:border-line-soft";
