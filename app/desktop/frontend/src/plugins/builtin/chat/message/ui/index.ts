// Message content rendering — the integrated whole that turns one Message
// into its rendered form: the message shell (avatar / header / outline /
// context menu), the markdown sub-module, and the content-block card renderers.
//
// This is the module's only public API. Everything else (BlockRenderer
// internals, MessageContextMenu, CitationContext, the markdown renderer +
// components, HitlCard) is private to the folder. The
// stream/panel chrome consumes MessageBlock; content-block plugins consume
// only the card renderers they register.

export { MessageBlock } from "./MessageBlock";
export { RootRunOutcome } from "./RootRunOutcome";
export type { BlockCtx } from "./BlockRenderer";
export { QuestionCard } from "./cards";
