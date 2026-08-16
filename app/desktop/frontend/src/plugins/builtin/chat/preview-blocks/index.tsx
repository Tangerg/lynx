import { contributeContentBlock, definePlugin, MESSAGE_CITATION_SOURCE } from "@/plugins/sdk";
import { searchCitations } from "./application/searchCitations";
import { CheckpointBlockRenderer, CodeBlockRenderer, SearchBlockRenderer } from "./ui/renderers";
import "./viewBlocks"; // side-effect: CustomContentBlockMap augmentation for the kinds below

export default definePlugin({
  name: "lyra.builtin.preview-blocks",
  setup(ctx) {
    contributeContentBlock(ctx, "search", SearchBlockRenderer);
    contributeContentBlock(ctx, "code", CodeBlockRenderer);
    contributeContentBlock(ctx, "checkpoint", CheckpointBlockRenderer);
    ctx.contribute(MESSAGE_CITATION_SOURCE, searchCitations);
  },
});
