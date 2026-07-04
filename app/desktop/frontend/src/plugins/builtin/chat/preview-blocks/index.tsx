import { definePlugin, MESSAGE_CITATION_SOURCE } from "@/plugins/sdk";
import { searchCitations } from "./application/searchCitations";
import { CheckpointBlockRenderer, CodeBlockRenderer, SearchBlockRenderer } from "./ui/renderers";
import "./viewBlocks"; // side-effect: CustomContentBlockMap augmentation for the kinds below

export default definePlugin({
  name: "lyra.builtin.preview-blocks",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("search", SearchBlockRenderer);
    host.message.registerContentBlock("code", CodeBlockRenderer);
    host.message.registerContentBlock("checkpoint", CheckpointBlockRenderer);
    host.extensions.contribute(MESSAGE_CITATION_SOURCE, searchCitations);
  },
});
