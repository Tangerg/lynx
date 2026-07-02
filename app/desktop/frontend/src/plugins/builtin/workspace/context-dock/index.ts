import type { ContextDockDestinationSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { CONTEXT_DOCK_DESTINATION } from "@/plugins/sdk/kernelPoints";

const destinations: ContextDockDestinationSpec[] = [
  {
    id: "search",
    title: "workspace.view.title.search",
    icon: "search",
    scope: "workspace",
    placement: "context-dock",
    order: 10,
  },
  {
    id: "explorer",
    title: "workspace.view.title.filetree",
    icon: "folder",
    scope: "workspace",
    placement: "context-dock",
    order: 20,
  },
  {
    id: "files",
    title: "workspace.view.title.files",
    icon: "filetext",
    scope: "workspace",
    placement: "context-dock",
    order: 30,
  },
  {
    id: "diff",
    title: "workspace.view.title.diff",
    icon: "diff",
    scope: "workspace",
    placement: "context-dock",
    order: 40,
  },
  {
    id: "codebase",
    title: "workspace.view.title.codebase",
    icon: "folder-search",
    scope: "workspace",
    placement: "context-dock",
    order: 50,
  },
  {
    id: "tools",
    title: "workspace.view.title.tools",
    icon: "tool",
    scope: "workspace",
    placement: "context-dock",
    order: 60,
  },
  {
    id: "skills",
    title: "workspace.view.title.skills",
    icon: "sparkle",
    scope: "workspace",
    placement: "context-dock",
    order: 70,
  },
  {
    id: "recipes",
    title: "workspace.view.title.recipes",
    icon: "book",
    scope: "workspace",
    placement: "context-dock",
    order: 80,
  },
  {
    id: "memory",
    title: "workspace.view.title.memory",
    icon: "filetext",
    scope: "workspace",
    placement: "context-dock",
    order: 90,
  },
  {
    id: "agent-docs",
    title: "workspace.view.title.agentDocs",
    icon: "book",
    scope: "workspace",
    placement: "context-dock",
    order: 100,
  },
  {
    id: "plan",
    title: "workspace.view.title.plan",
    icon: "list",
    scope: "run",
    placement: "context-dock",
    order: 110,
  },
  {
    id: "todos",
    title: "workspace.view.title.todos",
    icon: "check",
    scope: "run",
    placement: "context-dock",
    order: 120,
  },
  {
    id: "timeline",
    title: "workspace.view.title.timeline",
    icon: "history",
    scope: "session",
    placement: "context-dock",
    order: 130,
  },
  {
    id: "terminal",
    title: "workspace.view.title.terminal",
    icon: "terminal",
    scope: "workspace",
    placement: "context-dock",
    order: 140,
  },
];

export default definePlugin({
  name: "lyra.builtin.context-dock-destinations",
  version: "1.0.0",
  setup({ host }) {
    for (const destination of destinations) {
      host.extensions.contribute(CONTEXT_DOCK_DESTINATION, destination);
    }
  },
});
