// Renders plain text with file:line references (parseFileRefs) turned into
// clickable links that open the file viewer at the line (T2.3). Used for tool /
// command output where such refs are dense (build errors, grep hits, stack
// traces). A ref-free string renders as bare text (no wrapper spans).

import { useMemo } from "react";
import { parseFileRefs } from "@/plugins/builtin/agent/public/fileRefs";
import { FileRefLink } from "./FileRefLink";

export function LinkedText({ text }: { text: string }) {
  const segments = useMemo(() => parseFileRefs(text), [text]);
  if (segments.length === 1 && typeof segments[0] === "string") return text;
  return (
    <>
      {segments.map((seg, i) =>
        typeof seg === "string" ? seg : <FileRefLink key={i} path={seg.path} line={seg.line} />,
      )}
    </>
  );
}
