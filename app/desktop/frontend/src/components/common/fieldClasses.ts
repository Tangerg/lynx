// Shared chrome for inline text fields — the small path/rename inputs
// (relocate banner, add-project popover) and the memory editor textarea.
// Canvas-sunken, mono, accent focus ring; size, padding, and ink tone go
// at the call site via cn(). Same pattern as menuClasses.
export const FIELD_CLASSES =
  "rounded-md bg-canvas font-mono text-[12px] outline-none focus:ring-1 focus:ring-accent/40";

// Focus ring for the bordered settings-form text inputs (Providers / MCP /
// Connection panes): on focus the border goes accent and a soft 3px accent
// halo appears. Distinct from FIELD_CLASSES — that's the lighter ring-1
// treatment for inline canvas-sunken mini-inputs; this is the heavier ring for
// prominent form fields. Compose onto the input's own size/padding with cn().
export const INPUT_FOCUS_RING =
  "focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]";
