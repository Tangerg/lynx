export const CHAT_SEARCH_OPEN_EVENT = "lyra.chat-search.open";

export function openChatSearch() {
  window.dispatchEvent(new Event(CHAT_SEARCH_OPEN_EVENT));
}
