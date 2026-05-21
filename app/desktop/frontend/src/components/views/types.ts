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
