// Shared chrome for Radix menu surfaces (DropdownMenu / ContextMenu).
// className constants rather than a wrapper component because consumers
// sit on different Radix roots (DropdownMenu vs ContextMenu) — what they
// share is the look, not the behavior. Compose with cn() and append
// per-menu sizing (`min-w-*`, and `max-h-* overflow-auto` for scrolling
// lists — tailwind-merge lets the override win).

export const MENU_CONTENT_CLASSES =
  "z-50 overflow-hidden rounded-md bg-surface p-1 shadow-lg animate-rise-in";

// Item base: grid so a leading icon / trailing check slots in cleanly —
// callers append their `grid-cols-[…]` shape.
export const MENU_ITEM_CLASSES =
  "grid items-center gap-2 rounded-sm px-2.5 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg";
