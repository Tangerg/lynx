// The chat-beside-view split layout (G3): chat | resizer | view. Each pane is
// its own `relative flex-col` so ChatStream's absolute-positioned composer
// scopes to the chat half (not the whole Panel) and both halves scroll
// independently. The chat half's width is the persisted uiStore split ratio.

import type { ViewPlacement } from "./ViewPlacement";
import type { UserInput } from "@/lib/agent/composerInput";
import { ChatStream } from "./ChatStream";
import { SplitResizer } from "./SplitResizer";
import { ViewPlacementProvider } from "./ViewPlacement";
import { WorkspaceViewBody } from "./WorkspaceViewBody";

interface Props {
  onSend: (input: UserInput) => void;
  /** Active session — the chat half's reset key. */
  sessionId: string;
  /** The splittable workspace view shown in the right half. */
  viewId: string;
  /** Placement controls handed to the view's own header (close / etc.). */
  placement: ViewPlacement;
  /** Chat-half width as a fraction of the row (uiStore `splitRatio`). */
  ratio: number;
}

export function MainSplit({ onSend, sessionId, viewId, placement, ratio }: Props) {
  return (
    <div className="flex min-h-0 flex-1">
      <div
        className="relative flex min-h-0 min-w-0 flex-col"
        style={{ flexBasis: `${ratio * 100}%` }}
      >
        <ChatStream onSend={onSend} resetKey={sessionId} />
      </div>
      <SplitResizer />
      <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
        <ViewPlacementProvider value={placement}>
          <WorkspaceViewBody viewId={viewId} />
        </ViewPlacementProvider>
      </div>
    </div>
  );
}
