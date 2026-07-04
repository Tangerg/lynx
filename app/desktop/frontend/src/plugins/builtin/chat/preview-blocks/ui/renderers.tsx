import type { ContentBlockRenderer } from "@/plugins/sdk";
import { SearchResults } from "@/plugins/builtin/chat/tools/public/previews/SearchResults";
import { ShikiCodeBlock } from "@/ui";
import { Checkpoint } from "../Checkpoint";

export const SearchBlockRenderer: ContentBlockRenderer<"search"> = ({ block }) => (
  <SearchResults results={block.results} />
);

export const CodeBlockRenderer: ContentBlockRenderer<"code"> = ({ block }) => (
  <ShikiCodeBlock lang={block.lang} code={block.text} file={block.file} />
);

export const CheckpointBlockRenderer: ContentBlockRenderer<"checkpoint"> = ({ block }) => (
  <Checkpoint text={block.text} />
);
