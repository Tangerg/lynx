// Shared chrome for inline text fields — the small path/rename inputs
// (relocate banner, add-project popover) and the memory editor textarea.
// Canvas-sunken, mono, with a quiet `border-field` edge so the field reads as
// an editable surface (the skill keeps input outlines even where decorative
// borders are dropped); the edge strengthens on focus, no accent halo. Size,
// padding, and ink tone go at the call site via cn(); a seamless inline-rename
// callsite can opt the edge out with `border-0`.
export const FIELD_CLASSES =
  "rounded-md border-[0.5px] border-field bg-canvas font-mono text-[12px] outline-none transition-colors focus:border-field-strong";
