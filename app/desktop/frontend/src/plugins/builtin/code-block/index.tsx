// Built-in plugin: renderer for the `code` content block.

import { CodeBlock } from "@/components/chat/CodeBlock";
import { definePlugin, type ContentBlockRendererProps } from "@/plugins/sdk";

function CodeContentBlock({ block }: ContentBlockRendererProps<"code">) {
  return <CodeBlock lang={block.lang} file={block.file} text={block.text} />;
}

export default definePlugin({
  name: "lyra.builtin.code-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("code", CodeContentBlock);
  },
});
