// Built-in plugin: renderer for the `checkpoint` content block — a small
// horizontal-rule status marker (e.g. "Approved · running command").

import { Checkpoint } from "@/components/chat/Checkpoint";
import { definePlugin, type ContentBlockRendererProps } from "@/plugins/sdk";

function CheckpointContentBlock({ block }: ContentBlockRendererProps<"checkpoint">) {
  return <Checkpoint text={block.text} />;
}

export default definePlugin({
  name: "lyra.builtin.checkpoint-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("checkpoint", CheckpointContentBlock);
  },
});
