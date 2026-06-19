// Renders plain text with file:line references (parseFileRefs) turned into
// clickable links that open the file viewer at the line (T2.3). Used for tool /
// command output where such refs are dense (build errors, grep hits, stack
// traces). A ref-free string renders as bare text (no wrapper spans).

import { useMemo } from "react";
import { parseFileRefs } from "@/lib/agent/fileRefs";
import { useSessionStore } from "@/state/sessionStore";

export function LinkedText({ text }: { text: string }) {
  const segments = useMemo(() => parseFileRefs(text), [text]);
  const openFileViewer = useSessionStore((s) => s.openFileViewer);
  if (segments.length === 1 && typeof segments[0] === "string") return text;
  return (
    <>
      {segments.map((seg, i) =>
        typeof seg === "string" ? (
          seg
        ) : (
          <button
            key={i}
            type="button"
            onClick={() => openFileViewer(seg.path, seg.line)}
            className="cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-accent underline decoration-transparent transition-colors hover:decoration-current"
          >
            {seg.line > 0 ? `${seg.path}:${seg.line}` : seg.path}
          </button>
        ),
      )}
    </>
  );
}
