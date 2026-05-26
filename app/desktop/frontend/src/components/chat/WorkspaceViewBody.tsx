// WorkspaceViewBody — resolves a workspace view id to its body
// component via the plugin registry and renders it inside a
// PluginBoundary. Used when the user has promoted a workspace view
// (Settings, Diff, Files, …) into the chat-area tab strip.

import { PluginBoundary } from "@/plugins/PluginBoundary";
import { useWorkspaceViews } from "@/plugins/sdk";

interface Props {
  viewId: string;
}

export function WorkspaceViewBody({ viewId }: Props) {
  const workspaceViews = useWorkspaceViews();
  const Body = workspaceViews.find((v) => v.id === viewId)?.component ?? null;
  if (!Body) return null;
  return (
    <PluginBoundary plugin={`workspace:${viewId}`} label="main view">
      <Body />
    </PluginBoundary>
  );
}
