// Built-in plugin: renderer for the `approval` content block — the inline
// gate that asks the user to allow / skip a side-effectful command.

import { ApprovalCard } from "@/components/chat/ApprovalCard";
import { definePlugin, type ContentBlockRendererProps } from "@/plugins/sdk";

function ApprovalContentBlock({ block }: ContentBlockRendererProps<"approval">) {
  return <ApprovalCard what={block.text} cmd={block.command} reason={block.reason} />;
}

export default definePlugin({
  name: "lyra.builtin.approval-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("approval", ApprovalContentBlock);
  },
});
