// Message content rendering — the integrated whole that turns one Message
// into its rendered form: the message shell (avatar / header / outline /
// context menu), the per-block dispatcher (renderBlock), the markdown
// sub-module, and the content-block card renderers.
//
// This is the module's only public API. Everything else (BlockRenderer
// internals, MessageContextMenu, CitationContext, the markdown renderer +
// components, HitlCard) is private to the folder. The
// stream/panel chrome consumes MessageBlock + renderBlock; the content-block
// plugins consume the card renderers + ShikiCodeBlock.

export { MessageBlock } from "./MessageBlock";
export { renderBlock, type BlockCtx } from "./BlockRenderer";
export { ShikiCodeBlock } from "./markdown";
export {
  ApprovalCard,
  QuestionCard,
  ReasoningBlock,
  PlanBlock,
  PlanCheck,
  planItemRow,
} from "./cards";
