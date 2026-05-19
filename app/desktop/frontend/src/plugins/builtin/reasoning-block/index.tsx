// Built-in plugin: renderer for the `reasoning` content block — the
// collapsible "thinking" panel.

import { ReasoningBlock } from "@/components/chat/ReasoningBlock";
import { definePlugin, type ContentBlockRendererProps } from "@/plugins/sdk";

function ReasoningContentBlock({ block }: ContentBlockRendererProps<"reasoning">) {
  return <ReasoningBlock text={block.text} streaming={block.streaming} />;
}

export default definePlugin({
  name: "lyra.builtin.reasoning-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("reasoning", ReasoningContentBlock);
  },
});
