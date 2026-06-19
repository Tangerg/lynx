// A single clickable file:line reference — opens the file viewer at the line.
// Shared by the tool-output LinkedText (dense refs: build errors, grep hits)
// and the markdown-prose linkifier (rehypeFileRefs → markdownComponents `a`),
// so a file reference looks and behaves identically wherever it appears. That
// sameness is the point of sharing one component: divergent styling would read
// as two different kinds of link.

import { useSessionStore } from "@/state/sessionStore";

export function FileRefLink({ path, line }: { path: string; line: number }) {
  const openFileViewer = useSessionStore((s) => s.openFileViewer);
  return (
    <button
      type="button"
      onClick={() => openFileViewer(path, line)}
      className="cursor-pointer border-0 bg-transparent p-0 font-[inherit] text-accent underline decoration-transparent transition-colors hover:decoration-current"
    >
      {line > 0 ? `${path}:${line}` : path}
    </button>
  );
}
