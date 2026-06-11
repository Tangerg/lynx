// Shared chrome for inline text fields — the small path/rename inputs
// (relocate banner, add-project popover) and the memory editor textarea.
// Canvas-sunken, mono, accent focus ring; size, padding, and ink tone go
// at the call site via cn(). Same pattern as menuClasses.
export const FIELD_CLASSES =
  "rounded-md bg-canvas font-mono text-[12px] outline-none focus:ring-1 focus:ring-accent/40";
