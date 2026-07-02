import type { ContextDockDestinationSpec } from "@/plugins/sdk";

type BuiltinContextDockDestination = Omit<ContextDockDestinationSpec, "placement">;

function destination(spec: BuiltinContextDockDestination): ContextDockDestinationSpec {
  return { ...spec, placement: "context-dock" };
}

export const builtinContextDockDestinations: ContextDockDestinationSpec[] = [
  destination({
    id: "search",
    title: "workspace.view.title.search",
    icon: "search",
    scope: "workspace",
    order: 10,
  }),
  destination({
    id: "explorer",
    title: "workspace.view.title.filetree",
    icon: "folder",
    scope: "workspace",
    order: 20,
  }),
  destination({
    id: "files",
    title: "workspace.view.title.files",
    icon: "filetext",
    scope: "workspace",
    order: 30,
  }),
  destination({
    id: "diff",
    title: "workspace.view.title.diff",
    icon: "diff",
    scope: "workspace",
    order: 40,
  }),
  destination({
    id: "codebase",
    title: "workspace.view.title.codebase",
    icon: "folder-search",
    scope: "workspace",
    order: 50,
  }),
  destination({
    id: "tools",
    title: "workspace.view.title.tools",
    icon: "tool",
    scope: "workspace",
    order: 60,
  }),
  destination({
    id: "skills",
    title: "workspace.view.title.skills",
    icon: "sparkle",
    scope: "workspace",
    order: 70,
  }),
  destination({
    id: "recipes",
    title: "workspace.view.title.recipes",
    icon: "book",
    scope: "workspace",
    order: 80,
  }),
  destination({
    id: "memory",
    title: "workspace.view.title.memory",
    icon: "filetext",
    scope: "workspace",
    order: 90,
  }),
  destination({
    id: "agent-docs",
    title: "workspace.view.title.agentDocs",
    icon: "book",
    scope: "workspace",
    order: 100,
  }),
  destination({
    id: "plan",
    title: "workspace.view.title.plan",
    icon: "list",
    scope: "run",
    order: 110,
  }),
  destination({
    id: "todos",
    title: "workspace.view.title.todos",
    icon: "check",
    scope: "run",
    order: 120,
  }),
  destination({
    id: "timeline",
    title: "workspace.view.title.timeline",
    icon: "history",
    scope: "session",
    order: 130,
  }),
  destination({
    id: "terminal",
    title: "workspace.view.title.terminal",
    icon: "terminal",
    scope: "workspace",
    order: 140,
  }),
];
