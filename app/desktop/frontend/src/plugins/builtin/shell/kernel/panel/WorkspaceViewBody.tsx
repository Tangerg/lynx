// WorkspaceViewBody — resolves a workspace view id to its body
// component via the plugin registry and renders it inside a
// PluginBoundary. Used when the user has promoted a workspace view
// (Settings, Diff, Files, …) into the chat-area tab strip.

import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { PluginBoundary } from "@/plugins/host/PluginBoundary";
import { useWorkspaceViews } from "@/plugins/sdk";

interface Props {
  viewId: string;
}

export function WorkspaceViewBody({ viewId }: Props) {
  const t = useT();
  const workspaceViews = useWorkspaceViews();
  const Body = workspaceViews.find((v) => v.id === viewId)?.component;
  if (!Body) {
    // The header tab strip mirrors the store 1:1, so a view whose plugin
    // unloaded while its tab was active still shows a tab. Render a fallback
    // instead of a blank pane (a returned null here reads as a dead tab).
    return (
      <EmptyState
        icon="alert"
        title={t("workspace.view.unavailable.title")}
        sub={t("workspace.view.unavailable.body", { id: viewId })}
      />
    );
  }
  return (
    <PluginBoundary plugin={`workspace:${viewId}`} label="main view">
      <Body />
    </PluginBoundary>
  );
}
