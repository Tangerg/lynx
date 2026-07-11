import { createSingletonPort } from "@/lib/ports/singletonPort";
export type MessageStyle = "bubble" | "plain";
export type StreamReveal = "smooth" | "typewriter";

export interface PersonalizationPreferencesPort {
  useMessageStyle(): MessageStyle;
  useSetMessageStyle(): (style: MessageStyle) => void;
  useCompletionSound(): boolean;
  useSetCompletionSound(): (on: boolean) => void;
  useStreamReveal(): StreamReveal;
  useSetStreamReveal(): (mode: StreamReveal) => void;
}

const port = createSingletonPort<PersonalizationPreferencesPort>(
  "Personalization preferences port is not configured",
);

export const configurePersonalizationPreferencesPort = port.configure;
export const personalizationPreferences = port.get;
