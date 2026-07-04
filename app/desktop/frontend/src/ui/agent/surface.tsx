import type { ComponentPropsWithoutRef } from "react";
import { Surface } from "@/ui";

export function AgentSurface(props: ComponentPropsWithoutRef<typeof Surface>) {
  return <Surface {...props} />;
}
