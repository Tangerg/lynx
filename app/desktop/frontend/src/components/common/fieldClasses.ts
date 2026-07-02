// Shared chrome for inline text fields — the small path/rename inputs
// (relocate banner, add-project popover) and the memory editor textarea.
// Canvas-sunken, mono, with a quiet `border-field` edge so the field reads as
// an editable surface (the skill keeps input outlines even where decorative
// borders are dropped); the edge strengthens on focus, no accent halo. Size,
// padding, and ink tone go at the call site via cn(); a seamless inline-rename
// callsite can opt the edge out with `border-0`.
export const FIELD_CLASSES =
  "rounded-md border border-field bg-canvas font-mono text-[12px] outline-none transition-colors focus:border-field-strong";

// Focus treatment for the bordered settings-form text inputs (Providers / MCP /
// Connection panes): the `border-field` edge quietly strengthens on focus — no
// accent halo (that bright ring was one of the "cheap" tells). Compose onto the
// input's own size/padding with cn().
export const INPUT_FOCUS_RING = "focus:border-field-strong";
