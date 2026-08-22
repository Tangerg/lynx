import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { flattenMarkdown, flattenText } from "@/plugins/builtin/agent/public/messageContent";

export interface MessageCopyPayloads {
  markdown: string;
  plain: string;
  canCopy: boolean;
}

export function messageCopyPayloads(msg: Message): MessageCopyPayloads {
  const markdown = flattenMarkdown(msg.blocks);
  const plain = flattenText(msg.blocks);
  return {
    markdown,
    plain,
    canCopy: Boolean(markdown || plain),
  };
}
