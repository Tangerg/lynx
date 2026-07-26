import type { CSSProperties } from "react";

// How wide the chat column is when a workspace view sits beside it.
//
// A custom property on the row rather than a React style on the column: the
// resizer writes it on every pointer-move, and the column's `flex-basis` follows
// it in CSS with no render involved. The stored ratio only reaches React on
// release, so a drag costs one re-render instead of one per pointer event.
export const CHAT_SPLIT_PROPERTY = "--chat-split";

/** Row style carrying the persisted ratio — the drag's starting point. */
export function chatSplitRow(ratio: number): CSSProperties {
  return { [CHAT_SPLIT_PROPERTY]: `${ratio * 100}%` } as CSSProperties;
}

/** Column style, constant: the width lives in the property above. */
export const CHAT_SPLIT_COLUMN: CSSProperties = { flexBasis: `var(${CHAT_SPLIT_PROPERTY})` };
