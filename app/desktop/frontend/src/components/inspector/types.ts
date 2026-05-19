// Inspector-tab id type. Was a fixed string union (5 built-ins); now a plain
// string since plugins can contribute arbitrary ids.
export type InspectorTab = string;

export type FileChange = {
  path: string;
  change: "add" | "mod" | "del";
  added: number;
  removed: number;
};

export type MCPServer = {
  id: string;
  name: string;
  desc: string;
  tools: number;
  status: "active" | "idle" | "error";
  icon: string;
};
