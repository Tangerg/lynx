// Built-in plugin: appends a `search` content block when the agent reports
// web-search results (CUSTOM event `lyra.search-results`).

import { appendBlockToMessage, definePlugin } from "@/plugins/sdk";
import { CUSTOM, type SearchResultsPayload } from "@/protocol/agui/customEvents";

export default definePlugin({
  name: "lyra.builtin.search-results-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<SearchResultsPayload>(CUSTOM.SEARCH_RESULTS, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "search",
        toolCallId: value.parentMessageId,
        results: value.results,
      }),
    );
  },
});
