// Per-message action buttons in `message.actions` — four icon-only plugins
// (copy + edit + regenerate + feedback), each in its own file so a fork can
// drop one without touching the others. Shared chrome/helpers live in
// _shared.ts. This barrel only re-exports the plugin specs for the manifest.

export { messageCopy } from "./copy";
export { messageEdit } from "./edit";
export { messageRegenerate } from "./regenerate";
export { messageFeedback } from "./feedback";
