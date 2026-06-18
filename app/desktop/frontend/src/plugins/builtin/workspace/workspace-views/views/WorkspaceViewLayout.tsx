// The chrome every workspace-view tab shares: a ViewHeader over a scrolling
// body. Factored out so the 8 built-in views (and any plugin view) declare just
// their header + body, and the "header on top, scroll below" structure lives in
// one place. Body content (DataView, EmptyState, a raw list…) is the children.

import type { ReactNode, Ref } from "react";
import { ScrollArea } from "@/components/common";
import { ViewHeader, type ViewHeaderProps } from "./ViewHeader";

interface Props extends ViewHeaderProps {
  /** Extra classes on the scroll container (e.g. "py-1" for inset rows). */
  scrollClassName?: string;
  /** Ref to the scroll container — lets a view drive its own scroll position
   *  (the Diff view anchors to the bottom on open). */
  scrollRef?: Ref<HTMLDivElement>;
  children: ReactNode;
}

export function WorkspaceViewLayout({ scrollClassName, scrollRef, children, ...header }: Props) {
  return (
    <>
      <ViewHeader {...header} />
      <ScrollArea ref={scrollRef} className={scrollClassName}>
        {children}
      </ScrollArea>
    </>
  );
}
