// Built-in plugin: renderer for the `search` content block (web-search citation cards).

import { SearchResults } from "@/components/chat/SearchResults";
import { definePlugin, type ContentBlockRendererProps } from "@/plugins/sdk";

function SearchContentBlock({ block }: ContentBlockRendererProps<"search">) {
  return <SearchResults results={block.results} />;
}

export default definePlugin({
  name: "lyra.builtin.search-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("search", SearchContentBlock);
  },
});
