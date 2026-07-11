import { useMCPTools } from "./mcpServerQueries";

export function useMCPServerTools(server: string) {
  return useMCPTools({ server });
}
