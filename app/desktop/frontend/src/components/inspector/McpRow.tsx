import { Icon, type IconName } from "@/components/common";
import type { MCPServer } from "./types";

export function McpRow({ server }: { server: MCPServer }) {
  const label = server.status === "active" ? "On" : server.status === "idle" ? "Idle" : "Error";
  return (
    <div className={`mcp-row ${server.status}`}>
      <div className="mcp-icon"><Icon name={server.icon as IconName} size={15} /></div>
      <div style={{ minWidth: 0 }}>
        <div className="mcp-name">{server.name}</div>
        <div className="mcp-desc">{server.desc}</div>
      </div>
      <div className="mcp-tools">{server.tools} tools</div>
      <div className={`mcp-status ${server.status}`}>{label}</div>
    </div>
  );
}
