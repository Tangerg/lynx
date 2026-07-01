import { useMCPTools } from "@/lib/data/queries";

export function useMCPServerTools(server: string) {
  return useMCPTools({ server });
}
